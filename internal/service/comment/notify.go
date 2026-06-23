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
	// 自己评论自己的对象不产生通知（根对象作者就是评论者）。
	if aggregate.OwnerUserID != 0 && aggregate.Comment.UserID == aggregate.OwnerUserID {
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

// notifyCommentLiked 一级评论被点赞后发布 comment_liked 事件，接收人为评论作者。
func (s *commentService) notifyCommentLiked(targetType string, commentID uint, result *commentrepo.LikeResult, actorID uint) {
	if s.publisher == nil || result == nil || result.TargetUserID == 0 {
		return
	}
	// 自己点赞自己的评论不产生通知。
	if actorID == result.TargetUserID {
		return
	}

	_, _ = s.publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:        notificationservice.EventTypeCommentLiked,
		ActorUserID: &actorID,
		SourceType:  "comment",
		SourceID:    commentID,
		RootType:    targetType,
		RootID:      result.RootID,
		Metadata:    notificationservice.BuildRecipientMetadata(result.TargetUserID),
	})
}

// notifyReplyCreated 回复成功后发布 reply_created 事件，接收人为被回复人。
func (s *commentService) notifyReplyCreated(targetType string, aggregate *commentrepo.ReplyAggregate) {
	if s.publisher == nil || aggregate == nil {
		return
	}
	// 自己回复自己不产生通知。
	if aggregate.Reply.ToUserID != 0 && aggregate.Reply.FromUserID == aggregate.Reply.ToUserID {
		return
	}

	actorID := aggregate.Reply.FromUserID
	metadata := notificationservice.BuildReplyCreatedMetadata(
		aggregate.Reply.ToUserID,
		aggregate.Reply.CommentID,
		aggregate.QuotedContent,
	)

	_, _ = s.publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:           notificationservice.EventTypeReplyCreated,
		ActorUserID:    &actorID,
		SourceType:     "reply",
		SourceID:       aggregate.Reply.ID,
		RootType:       targetType,
		RootID:         aggregate.TargetID,
		ContentExcerpt: aggregate.Reply.Content,
		Metadata:       metadata,
	})
}

// notifyReplyLiked 回复被点赞后发布 reply_liked 事件，接收人为回复作者。
func (s *commentService) notifyReplyLiked(targetType string, replyID uint, result *commentrepo.LikeResult, actorID uint) {
	if s.publisher == nil || result == nil || result.TargetUserID == 0 || result.RootID == 0 {
		return
	}
	// 自己点赞自己的回复不产生通知。
	if actorID == result.TargetUserID {
		return
	}

	_, _ = s.publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:        notificationservice.EventTypeReplyLiked,
		ActorUserID: &actorID,
		SourceType:  "reply",
		SourceID:    replyID,
		RootType:    targetType,
		RootID:      result.RootID,
		Metadata: notificationservice.BuildReplyLikedMetadata(
			result.TargetUserID,
			result.RootID,
		),
	})
}
