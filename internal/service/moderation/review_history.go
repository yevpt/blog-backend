package moderation

import (
	"context"
	"time"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
)

// ReviewHistoryCommand 定位审核项及历史分页。
type ReviewHistoryCommand struct {
	ItemID   uint64
	Page     int
	PageSize int
}

// ReviewHistoryImage 是历史修订中的有序图片快照。
type ReviewHistoryImage struct {
	Seq       uint
	ObjectKey string
	MediaType string
	IsGIF     bool
}

// ReviewHistoryEvent 是管理端可读取的审核操作事件。
type ReviewHistoryEvent struct {
	ID           uint64
	RevisionID   *uint64
	ActorUserID  *uint64
	Action       Event
	Reason       *string
	MetadataJSON *string
	CreatedAt    time.Time
}

// ReviewHistoryPage 聚合一页历史修订、对应图片和审核项操作事件。
type ReviewHistoryPage struct {
	Total     int64
	Page      int
	PageSize  int
	Revisions []ReviewItem
	Images    map[uint64][]ReviewHistoryImage
	Events    []ReviewHistoryEvent
}

// History 分页返回审核项的不可变修订及审计事实。
func (s *reviewService) History(ctx context.Context, cmd ReviewHistoryCommand) (ReviewHistoryPage, error) {
	if s.repo == nil || cmd.ItemID == 0 {
		return ReviewHistoryPage{}, ErrInvalidRequest
	}
	page, pageSize := s.normalizeReviewPage(cmd.Page, cmd.PageSize)
	if pageSize > 100 {
		pageSize = 100
	}
	result, err := s.repo.LoadReviewHistory(ctx, cmd.ItemID, page, pageSize)
	if err != nil {
		return ReviewHistoryPage{}, mapReviewRepositoryError(err)
	}
	return reviewHistoryPageFromRepository(result), nil
}

func reviewHistoryPageFromRepository(page moderationrepo.ReviewHistoryPage) ReviewHistoryPage {
	revisions := make([]ReviewItem, 0, len(page.Revisions))
	for _, revision := range page.Revisions {
		revisions = append(revisions, reviewItemFromRecord(revision))
	}
	images := make(map[uint64][]ReviewHistoryImage, len(page.Images))
	for revisionID, records := range page.Images {
		mapped := make([]ReviewHistoryImage, 0, len(records))
		for _, image := range records {
			mapped = append(mapped, ReviewHistoryImage{
				Seq: image.Seq, ObjectKey: image.ObjectKey, MediaType: image.MediaType, IsGIF: image.IsGIF,
			})
		}
		images[revisionID] = mapped
	}
	events := make([]ReviewHistoryEvent, 0, len(page.Events))
	for _, event := range page.Events {
		events = append(events, ReviewHistoryEvent{
			ID: event.ID, RevisionID: event.RevisionID, ActorUserID: event.ActorUserID,
			Action: Event(event.Action), Reason: event.Reason, MetadataJSON: event.MetadataJSON, CreatedAt: event.CreatedAt,
		})
	}
	return ReviewHistoryPage{
		Total: page.Total, Page: page.Page, PageSize: page.PageSize,
		Revisions: revisions, Images: images, Events: events,
	}
}
