package notification

import (
	"context"
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
)

// Publish 校验事件类型，对标题与摘要做去空白和定长快照，落库为待分发事件。
func (s *publisherService) Publish(ctx context.Context, event PublishEvent) (*model.NotificationEvent, error) {
	// 类型必须在允许集合内，避免脏类型流入分发与邮件链路。
	if _, ok := allowedEventTypes[event.Type]; !ok {
		return nil, ErrInvalidEventType
	}

	// 标题与摘要去首尾空白后按列宽截断，保存创建时刻的快照。
	now := time.Now()
	record := &model.NotificationEvent{
		Type:           event.Type,
		ActorUserID:    event.ActorUserID,
		SourceType:     event.SourceType,
		SourceID:       event.SourceID,
		RootType:       event.RootType,
		RootID:         event.RootID,
		Title:          truncateRunes(strings.TrimSpace(event.Title), maxTitleRunes),
		ContentExcerpt: truncateRunes(strings.TrimSpace(event.ContentExcerpt), maxExcerptRunes),
		MetadataJSON:   event.Metadata,
		DispatchStatus: notificationrepo.EventStatusPending,
		NextProcessAt:  now,
	}

	if err := s.repo.CreateEvent(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

// truncateRunes 按 rune 截断字符串，避免在多字节字符中间截断。
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
