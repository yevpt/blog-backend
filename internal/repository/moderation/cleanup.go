package moderation

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// CleanupRepository 是定期清理 worker 使用的有界数据边界。
type CleanupRepository interface {
	CleanupAudit(ctx context.Context, cmd AuditCleanupCommand) (AuditCleanupResult, error)
	ListStaleImages(ctx context.Context, before time.Time, limit int) ([]StaleImageRecord, error)
	DeleteStaleImage(ctx context.Context, sha256 string, size uint64, lastUsedBefore time.Time) (bool, error)
	ReferencedObjectKeys(ctx context.Context, keys []string) (map[string]struct{}, error)
}

// AuditCleanupCommand 定义三类审核记录的独立截止时间和批次上限。
type AuditCleanupCommand struct {
	AttemptBefore   time.Time
	ActionLogBefore time.Time
	RevisionBefore  time.Time
	Limit           int
}

// AuditCleanupResult 返回本轮数据库清理数量。
type AuditCleanupResult struct {
	Attempts   int64
	ActionLogs int64
	Revisions  int64
}

// StaleImageRecord 是无版本引用且长期未访问的图片审核记录。
type StaleImageRecord struct {
	SHA256           string
	Size             uint64
	PreviewObjectKey *string
	LastUsedAt       time.Time
}

// NewCleanupRepository 构造审核定期清理仓储。
func NewCleanupRepository(db *gorm.DB) CleanupRepository { return &repository{db: db} }

// CleanupAudit 在单事务内按相同批次上限删除过期审计记录和无引用旧版本。
func (r *repository) CleanupAudit(ctx context.Context, cmd AuditCleanupCommand) (AuditCleanupResult, error) {
	if cmd.Limit <= 0 || cmd.AttemptBefore.IsZero() || cmd.ActionLogBefore.IsZero() || cmd.RevisionBefore.IsZero() {
		return AuditCleanupResult{}, ErrInvalidCommand
	}
	var result AuditCleanupResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempts := tx.WithContext(ctx).Exec(
			"DELETE FROM moderation_attempt WHERE created_at < ? ORDER BY id LIMIT ?", cmd.AttemptBefore, cmd.Limit)
		if attempts.Error != nil {
			return attempts.Error
		}
		result.Attempts = attempts.RowsAffected

		logs := tx.WithContext(ctx).Exec(
			"DELETE FROM moderation_action_log WHERE created_at < ? ORDER BY id LIMIT ?", cmd.ActionLogBefore, cmd.Limit)
		if logs.Error != nil {
			return logs.Error
		}
		result.ActionLogs = logs.RowsAffected

		var revisionIDs []uint64
		if err := tx.WithContext(ctx).Raw(`SELECT id FROM moderation_revision
WHERE created_at < ?
  AND review_status IN ('rejected','superseded')
  AND NOT EXISTS (
    SELECT 1 FROM moderation_item
    WHERE materialized_revision_id = moderation_revision.id
       OR approved_revision_id = moderation_revision.id
       OR pending_revision_id = moderation_revision.id
  )
  AND NOT EXISTS (
    SELECT 1 FROM moderation_action_log
    WHERE revision_id = moderation_revision.id
  )
ORDER BY id LIMIT ? FOR UPDATE`, cmd.RevisionBefore, cmd.Limit).Scan(&revisionIDs).Error; err != nil {
			return err
		}
		if len(revisionIDs) == 0 {
			return nil
		}
		if err := tx.WithContext(ctx).Where("revision_id IN ?", revisionIDs).
			Delete(&model.ModerationRevisionImage{}).Error; err != nil {
			return err
		}
		revisions := tx.WithContext(ctx).Where("id IN ?", revisionIDs).Delete(&model.ModerationRevision{})
		if revisions.Error != nil {
			return revisions.Error
		}
		result.Revisions = revisions.RowsAffected
		return nil
	})
	return result, err
}

// ListStaleImages 查询无任何版本引用的过期图片审核记录。
func (r *repository) ListStaleImages(ctx context.Context, before time.Time, limit int) ([]StaleImageRecord, error) {
	if before.IsZero() || limit <= 0 {
		return nil, ErrInvalidCommand
	}
	var rows []StaleImageRecord
	err := r.db.WithContext(ctx).Table("moderation_image").
		Select("sha256", "size", "preview_object_key", "last_used_at").
		Where("last_used_at < ?", before).
		Where("NOT EXISTS (SELECT 1 FROM moderation_revision_image WHERE moderation_revision_image.sha256 = moderation_image.sha256 AND moderation_revision_image.size = moderation_image.size)").
		Order("last_used_at ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// DeleteStaleImage 使用最后访问截止时间二次校验，避免并发复用后误删记录。
func (r *repository) DeleteStaleImage(ctx context.Context, sha256 string, size uint64, lastUsedBefore time.Time) (bool, error) {
	if sha256 == "" || size == 0 || lastUsedBefore.IsZero() {
		return false, ErrInvalidCommand
	}
	result := r.db.WithContext(ctx).Where("sha256 = ? AND size = ? AND last_used_at < ?", sha256, size, lastUsedBefore).
		Delete(&model.ModerationImage{})
	return result.RowsAffected == 1, result.Error
}

// ReferencedObjectKeys 返回仍由审核图片记录或版本快照引用的对象 key。
func (r *repository) ReferencedObjectKeys(ctx context.Context, keys []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if len(keys) == 0 {
		return result, nil
	}
	var rows []string
	err := r.db.WithContext(ctx).Raw(`SELECT preview_object_key AS object_key
FROM moderation_image WHERE preview_object_key IN ?
UNION
SELECT object_key FROM moderation_revision_image WHERE object_key IN ?`, keys, keys).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, key := range rows {
		result[key] = struct{}{}
	}
	return result, nil
}
