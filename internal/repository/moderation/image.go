package moderation

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
)

// UseApprovedImage 使用完整指纹命中已通过图片，并刷新最后访问时间。
func (r *repository) UseApprovedImage(ctx context.Context, fingerprint ImageFingerprint, usedAt time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Exec(
		"UPDATE moderation_image SET last_used_at = ?, updated_at = ? WHERE md5 = ? AND sha256 = ? AND size = ? AND status = ?",
		usedAt, usedAt, fingerprint.MD5, fingerprint.SHA256, fingerprint.Size, ImageApproved,
	)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// UpsertPendingImage 幂等登记待审图片，已通过记录不会被降级或恢复预览。
func (r *repository) UpsertPendingImage(ctx context.Context, image PendingImage) error {
	return r.db.WithContext(ctx).Exec(`
INSERT INTO moderation_image
  (sha256, size, md5, status, preview_object_key, last_used_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  md5 = VALUES(md5),
  status = IF(status = 'approved', status, VALUES(status)),
  preview_object_key = IF(status = 'approved', preview_object_key, VALUES(preview_object_key)),
  last_used_at = VALUES(last_used_at),
  updated_at = VALUES(updated_at)`,
		image.Fingerprint.SHA256, image.Fingerprint.Size, image.Fingerprint.MD5, ImagePending,
		image.PreviewObjectKey, image.LastUsedAt, image.LastUsedAt, image.LastUsedAt,
	).Error
}

// LoadRevisionImages 按提交顺序读取版本图片快照。
func (r *repository) LoadRevisionImages(ctx context.Context, revisionID uint64) ([]RevisionImageRecord, error) {
	var rows []model.ModerationRevisionImage
	if err := r.db.WithContext(ctx).Where("revision_id = ?", revisionID).Order("seq ASC,id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]RevisionImageRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, RevisionImageRecord{
			ImageFingerprint: ImageFingerprint{SHA256: row.SHA256, MD5: row.MD5, Size: row.Size},
			Seq:              row.Seq, ObjectKey: row.ObjectKey, MediaType: row.MediaType, IsGIF: row.IsGIF,
		})
	}
	return result, nil
}
