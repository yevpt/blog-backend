package moderation

import (
	"context"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"go.uber.org/zap"
)

func (s *reviewService) loadReviewNotificationContext(
	ctx context.Context,
	record moderationrepo.ReviewRecord,
) moderationrepo.ReviewNotificationContext {
	fallback := moderationrepo.ReviewNotificationContext{ContentType: record.Subject.Type}
	if s.repo == nil {
		return fallback
	}
	subject, err := s.repo.LoadSubject(ctx, record.Subject)
	if err != nil {
		s.logger.Warn("加载审核通知主体失败，回退为仅展示内容类型",
			zap.String("content_type", string(record.Subject.Type)),
			zap.Uint64("content_id", record.Subject.ID),
			zap.Error(err),
		)
		return fallback
	}
	notifCtx, err := s.repo.LoadReviewNotificationContext(ctx, subject.Ref)
	if err != nil {
		s.logger.Warn("加载审核通知上下文失败，回退为仅展示内容类型",
			zap.String("content_type", string(subject.Ref.Type)),
			zap.Uint64("content_id", subject.Ref.ID),
			zap.Error(err),
		)
		return moderationrepo.ReviewNotificationContext{ContentType: subject.Ref.Type}
	}
	if notifCtx.ContentType == "" {
		notifCtx.ContentType = subject.Ref.Type
	}
	return notifCtx
}

func interactionNotification(
	subject moderationrepo.SubjectRef,
	actorUserID uint64,
	content string,
	notifCtx moderationrepo.ReviewNotificationContext,
) *moderationrepo.InteractionNotificationIntent {
	eventType, sourceType, rootType, rootID := interactionNotificationTarget(subject, notifCtx)
	if eventType == "" || notifCtx.InteractionRecipientUserID == 0 || rootID == 0 {
		return nil
	}
	return &moderationrepo.InteractionNotificationIntent{
		Type:            eventType,
		ActorUserID:     actorUserID,
		RecipientUserID: notifCtx.InteractionRecipientUserID,
		SourceType:      sourceType,
		RootType:        rootType,
		RootID:          rootID,
		ContentExcerpt:  content,
		CommentID:       notifCtx.CommentID,
		RootSnapshot:    notifCtx.RootSnapshot,
		QuoteSnapshot:   notifCtx.QuoteSnapshot,
	}
}

func interactionNotificationTarget(
	subject moderationrepo.SubjectRef,
	notifCtx moderationrepo.ReviewNotificationContext,
) (eventType, sourceType, rootType string, rootID uint64) {
	switch subject.Type {
	case moderationrepo.SubjectArticleComment:
		return "comment_created", "comment", "article", subject.RootID
	case moderationrepo.SubjectMomentComment:
		return "comment_created", "comment", "moment", subject.RootID
	case moderationrepo.SubjectGuestbook:
		return "guestbook_created", "guestbook", "guestbook", subject.ID
	case moderationrepo.SubjectArticleCommentReply:
		return "reply_created", "reply", "article", notificationRootID(notifCtx)
	case moderationrepo.SubjectMomentCommentReply:
		return "reply_created", "reply", "moment", notificationRootID(notifCtx)
	case moderationrepo.SubjectGuestbookReply:
		return "reply_created", "reply", "guestbook", notificationRootID(notifCtx)
	default:
		return "", "", "", 0
	}
}

func notificationRootID(notifCtx moderationrepo.ReviewNotificationContext) uint64 {
	if notifCtx.RootSnapshot == nil {
		return 0
	}
	return notifCtx.RootSnapshot.ID
}
