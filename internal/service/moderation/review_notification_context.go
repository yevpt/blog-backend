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
