package comment

import (
	"context"

	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// notifyCommentCreated 评论成功后发布 comment_created 事件。
// 留言板评论的接收人是板主（target.ID 即板主用户 ID），其余目标由分发器按根对象归属解析。
func (s *commentService) notifyCommentCreated(ctx context.Context, targetType string, targetID uint, aggregate *commentrepo.CommentAggregate) {
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
		metadata = notificationservice.BuildSourceRootMetadata(
			notificationservice.NotificationSnapshot{Type: "guestbook", ID: aggregate.Comment.ID, Excerpt: aggregate.Comment.Content},
			&notificationservice.NotificationSnapshot{Type: "guestbook", ID: aggregate.Comment.ID, Excerpt: aggregate.Comment.Content},
			targetID,
		)
	} else {
		metadata = notificationservice.BuildSourceRootMetadata(
			notificationservice.NotificationSnapshot{Type: "comment", ID: aggregate.Comment.ID, Excerpt: aggregate.Comment.Content},
			notificationRootSnapshot(aggregate.RootSnapshot),
		)
	}

	_, _ = s.publisher.Publish(ctx, notificationservice.PublishEvent{
		Type:           notificationservice.EventTypeCommentCreated,
		ActorUserID:    &actorID,
		SourceType:     "comment",
		SourceID:       aggregate.Comment.ID,
		RootType:       targetType,
		RootID:         commentCreatedRootID(targetType, targetID, aggregate),
		ContentExcerpt: aggregate.Comment.Content,
		Metadata:       metadata,
	})
}

func commentCreatedRootID(targetType string, targetID uint, aggregate *commentrepo.CommentAggregate) uint {
	if targetType == targetTypeGuestbook && aggregate != nil {
		return aggregate.Comment.ID
	}
	return targetID
}

// notifyCommentLiked 一级评论被点赞后发布 comment_liked 事件，接收人为评论作者。
func (s *commentService) notifyCommentLiked(ctx context.Context, targetType string, commentID uint, result *commentrepo.LikeResult, actorID uint) {
	if s.publisher == nil || result == nil || result.TargetUserID == 0 {
		return
	}
	// 自己点赞自己的评论不产生通知。
	if actorID == result.TargetUserID {
		return
	}

	_, _ = s.publisher.Publish(ctx, notificationservice.PublishEvent{
		Type:        notificationservice.EventTypeCommentLiked,
		ActorUserID: &actorID,
		SourceType:  "comment",
		SourceID:    commentID,
		RootType:    targetType,
		RootID:      result.RootID,
		Metadata: notificationservice.BuildSourceRootMetadata(
			notificationservice.NotificationSnapshot{Type: "comment", ID: commentID, Excerpt: result.TargetContent},
			notificationRootSnapshot(result.RootSnapshot),
			result.TargetUserID,
		),
	})
}

// notifyReplyCreated 回复成功后发布 reply_created 事件，接收人为被回复人。
func (s *commentService) notifyReplyCreated(ctx context.Context, targetType string, aggregate *commentrepo.ReplyAggregate) {
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
		aggregate.Reply.Content,
		notificationRootSnapshot(aggregate.RootSnapshot),
		aggregate.QuotedContent,
	)

	_, _ = s.publisher.Publish(ctx, notificationservice.PublishEvent{
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
func (s *commentService) notifyReplyLiked(ctx context.Context, targetType string, replyID uint, result *commentrepo.LikeResult, actorID uint) {
	if s.publisher == nil || result == nil || result.TargetUserID == 0 || result.RootID == 0 {
		return
	}
	// 自己点赞自己的回复不产生通知。
	if actorID == result.TargetUserID {
		return
	}

	_, _ = s.publisher.Publish(ctx, notificationservice.PublishEvent{
		Type:        notificationservice.EventTypeReplyLiked,
		ActorUserID: &actorID,
		SourceType:  "reply",
		SourceID:    replyID,
		RootType:    targetType,
		RootID:      result.RootID,
		Metadata: notificationservice.BuildReplyLikedMetadata(
			result.TargetUserID,
			result.CommentID,
			result.TargetContent,
			notificationRootSnapshot(result.RootSnapshot),
		),
	})
}

func notificationRootSnapshot(snapshot commentrepo.RootSnapshot) *notificationservice.NotificationSnapshot {
	if snapshot.Type == "" && snapshot.ID == 0 && snapshot.Title == "" && snapshot.Excerpt == "" {
		return nil
	}
	return &notificationservice.NotificationSnapshot{
		Type:    snapshot.Type,
		ID:      snapshot.ID,
		Title:   snapshot.Title,
		Excerpt: snapshot.Excerpt,
	}
}
