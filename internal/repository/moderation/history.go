package moderation

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type reviewHistoryCountRow struct {
	ItemID uint64
	Total  int64
}

type reviewHistoryImageRow struct {
	RevisionID uint64
	Seq        uint
	ObjectKey  string
	SHA256     string
	MD5        string
	Size       uint64
	MediaType  string
	IsGIF      bool
}

type reviewHistoryEventRow struct {
	ID           uint64
	RevisionID   *uint64
	ActorUserID  *uint64
	Action       string
	Reason       *string
	MetadataJSON *string
	CreatedAt    time.Time
}

// LoadReviewHistory 分页读取审核项修订，并批量加载当前页图片和操作事件。
func (r *repository) LoadReviewHistory(ctx context.Context, itemID uint64, page, pageSize int) (ReviewHistoryPage, error) {
	if itemID == 0 || page < 1 || pageSize < 1 || pageSize > 100 {
		return ReviewHistoryPage{}, ErrInvalidCommand
	}

	count, err := r.loadReviewHistoryCount(ctx, itemID)
	if err != nil {
		return ReviewHistoryPage{}, err
	}
	revisions, err := r.loadReviewHistoryRevisions(ctx, itemID, page, pageSize)
	if err != nil {
		return ReviewHistoryPage{}, err
	}
	images, err := r.loadReviewHistoryImages(ctx, revisions)
	if err != nil {
		return ReviewHistoryPage{}, err
	}
	events, err := r.loadReviewHistoryEvents(ctx, itemID)
	if err != nil {
		return ReviewHistoryPage{}, err
	}
	return ReviewHistoryPage{
		Total: count.Total, Page: page, PageSize: pageSize,
		Revisions: revisions, Images: images, Events: events,
	}, nil
}

func (r *repository) loadReviewHistoryCount(ctx context.Context, itemID uint64) (reviewHistoryCountRow, error) {
	var row reviewHistoryCountRow
	err := r.db.WithContext(ctx).Table("moderation_item").
		Select("moderation_item.id AS item_id,count(moderation_revision.id) AS total").
		Joins("LEFT JOIN moderation_revision ON moderation_revision.item_id = moderation_item.id").
		Where("moderation_item.id = ?", itemID).
		Group("moderation_item.id").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return reviewHistoryCountRow{}, ErrItemNotFound
	}
	return row, err
}

func (r *repository) loadReviewHistoryRevisions(ctx context.Context, itemID uint64, page, pageSize int) ([]ReviewRecord, error) {
	var rows []reviewRecordRow
	err := r.reviewQuery(ctx).Select(reviewRecordSelect).
		Where("moderation_item.id = ?", itemID).
		Order("moderation_revision.version DESC,moderation_revision.id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	revisions := make([]ReviewRecord, 0, len(rows))
	for _, row := range rows {
		revisions = append(revisions, mapReviewRecord(row))
	}
	return revisions, nil
}

func (r *repository) loadReviewHistoryImages(ctx context.Context, revisions []ReviewRecord) (map[uint64][]RevisionImageRecord, error) {
	images := make(map[uint64][]RevisionImageRecord, len(revisions))
	if len(revisions) == 0 {
		return images, nil
	}
	revisionIDs := make([]uint64, 0, len(revisions))
	for _, revision := range revisions {
		revisionIDs = append(revisionIDs, revision.RevisionID)
		images[revision.RevisionID] = []RevisionImageRecord{}
	}
	var rows []reviewHistoryImageRow
	err := r.db.WithContext(ctx).Table("moderation_revision_image").
		Select("revision_id,seq,object_key,sha256,md5,size,media_type,is_gif").
		Where("revision_id IN ?", revisionIDs).
		Order("revision_id ASC,seq ASC,id ASC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		images[row.RevisionID] = append(images[row.RevisionID], RevisionImageRecord{
			ImageFingerprint: ImageFingerprint{SHA256: row.SHA256, MD5: row.MD5, Size: row.Size},
			Seq:              row.Seq, ObjectKey: row.ObjectKey, MediaType: row.MediaType, IsGIF: row.IsGIF,
		})
	}
	return images, nil
}

func (r *repository) loadReviewHistoryEvents(ctx context.Context, itemID uint64) ([]ReviewHistoryEvent, error) {
	var rows []reviewHistoryEventRow
	err := r.db.WithContext(ctx).Table("moderation_action_log").
		Select("id,revision_id,actor_user_id,action,reason,metadata_json,created_at").
		Where("item_id = ?", itemID).Order("created_at ASC,id ASC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	events := make([]ReviewHistoryEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, ReviewHistoryEvent{
			ID: row.ID, RevisionID: row.RevisionID, ActorUserID: row.ActorUserID,
			Action: Event(row.Action), Reason: row.Reason, MetadataJSON: row.MetadataJSON, CreatedAt: row.CreatedAt,
		})
	}
	return events, nil
}
