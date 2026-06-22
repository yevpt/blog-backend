package moment

import (
	"context"

	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// notifyMomentLiked 发布 moment_liked 事件；点赞仅站内通知，不进邮件队列。
func (s *momentService) notifyMomentLiked(momentID uint, userID uint, content string) {
	if s.publisher == nil {
		return
	}
	actorID := userID
	_, _ = s.publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:           notificationservice.EventTypeMomentLiked,
		ActorUserID:    &actorID,
		SourceType:     "moment",
		SourceID:       momentID,
		RootType:       "moment",
		RootID:         momentID,
		ContentExcerpt: content,
	})
}
