package guestbook

import (
	"context"

	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// notifyGuestbookCreated 留言成功后发布 guestbook_created 事件，接收人显式指定为板主。
func (s *guestbookService) notifyGuestbookCreated(ownerUserID uint, fromUserID uint, aggregate *guestbookrepo.GuestbookAggregate) {
	if s.publisher == nil || aggregate == nil {
		return
	}
	// 自己在自己留言板留言不产生通知。
	if fromUserID == ownerUserID {
		return
	}
	actorID := fromUserID
	_, _ = s.publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:           notificationservice.EventTypeGuestbookCreated,
		ActorUserID:    &actorID,
		SourceType:     "guestbook",
		SourceID:       aggregate.Message.ID,
		RootType:       "guestbook",
		RootID:         aggregate.Message.ID,
		ContentExcerpt: aggregate.Message.Content,
		Metadata:       notificationservice.BuildRecipientMetadata(ownerUserID),
	})
}

// notifyGuestbookLiked 发布 guestbook_liked 事件；接收人由分发器按留言归属解析，点赞仅站内通知。
func (s *guestbookService) notifyGuestbookLiked(guestbookID uint, userID uint) {
	if s.publisher == nil {
		return
	}
	actorID := userID
	_, _ = s.publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:        notificationservice.EventTypeGuestbookLiked,
		ActorUserID: &actorID,
		SourceType:  "guestbook",
		SourceID:    guestbookID,
		RootType:    "guestbook",
		RootID:      guestbookID,
	})
}
