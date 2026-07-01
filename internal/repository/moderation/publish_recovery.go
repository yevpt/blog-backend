package moderation

import (
	"context"

	"gorm.io/gorm"
)

// PublishRecoveryCandidate 是已通过但图片尚未完成正式化的碎语。
type PublishRecoveryCandidate struct {
	ItemID             uint64
	RevisionID         uint64
	AuthorID           uint64
	MomentID           uint64
	PreviousRevisionID *uint64
}

// PublishRecoveryRepository 是图片正式化补偿 worker 的最小数据边界。
type PublishRecoveryRepository interface {
	ListPublishRecoveryCandidates(ctx context.Context, limit int) ([]PublishRecoveryCandidate, error)
	LoadRevisionImages(ctx context.Context, revisionID uint64) ([]RevisionImageRecord, error)
}

// NewPublishRecoveryRepository 构造图片正式化补偿仓储。
func NewPublishRecoveryRepository(db *gorm.DB) PublishRecoveryRepository { return &repository{db: db} }

// ListPublishRecoveryCandidates 查询已通过但仍处于占位态的碎语，并定位上一个通过版本。
func (r *repository) ListPublishRecoveryCandidates(ctx context.Context, limit int) ([]PublishRecoveryCandidate, error) {
	if r == nil || r.db == nil || limit <= 0 {
		return nil, ErrInvalidCommand
	}
	var rows []PublishRecoveryCandidate
	err := r.db.WithContext(ctx).Table("moderation_item AS item").
		Select(`item.id AS item_id, item.approved_revision_id AS revision_id,
			item.author_id, item.content_id AS moment_id,
			(SELECT previous.id FROM moderation_revision AS previous
			 WHERE previous.item_id = item.id
			   AND previous.review_status = 'approved'
			   AND previous.id <> item.approved_revision_id
			 ORDER BY previous.version DESC LIMIT 1) AS previous_revision_id`).
		Where("item.content_type = ?", string(SubjectMoment)).
		Where("item.lifecycle_state = ?", string(LifecycleActive)).
		Where("item.public_state = ?", string(PublicPlaceholder)).
		Where("item.approved_revision_id IS NOT NULL").
		Order("item.updated_at ASC, item.id ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}
