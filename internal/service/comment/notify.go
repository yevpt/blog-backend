package comment

import (
	"context"

	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// notifyCommentCreated 评论成功后发布 comment_created 事件。
// 留言板评论的接收人是板主（target.ID 即板主用户 ID），其余目标由分发器按根对象归属解析。
func (s *commentService) notifyCommentCreated(targetType string, targetID uint, aggregate *commentrepo.CommentAggregate) {
	if s.publisher == nil || aggregate == nil {
		return
	}

	actorID := aggregate.Comment.UserID
	var metadata *string
	if targetType == targetTypeGuestbook {
		metadata = notificationservice.BuildRecipientMetadata(targetID)
	}

	_, _ = s.publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:           notificationservice.EventTypeCommentCreated,
		ActorUserID:    &actorID,
		SourceType:     "comment",
		SourceID:       aggregate.Comment.ID,
		RootType:       targetType,
		RootID:         targetID,
		ContentExcerpt: aggregate.Comment.Content,
		Metadata:       metadata,
	})
}

// notifyReplyCreated 回复成功后发布 reply_created 事件，接收人为被回复人。
func (s *commentService) notifyReplyCreated(targetType string, commentID uint, aggregate *commentrepo.ReplyAggregate) {
	if s.publisher == nil || aggregate == nil {
		return
	}

	actorID := aggregate.Reply.FromUserID
	var metadata *string
	if aggregate.Reply.ToUserID != 0 {
		metadata = notificationservice.BuildRecipientMetadata(aggregate.Reply.ToUserID)
	}

	_, _ = s.publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:           notificationservice.EventTypeReplyCreated,
		ActorUserID:    &actorID,
		SourceType:     "reply",
		SourceID:       aggregate.Reply.ID,
		RootType:       targetType,
		RootID:         commentID,
		ContentExcerpt: aggregate.Reply.Content,
		Metadata:       metadata,
	})
}
