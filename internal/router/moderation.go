package router

import (
	"context"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// maybeNewModerationService 在审核开启时加载规则并构造运行时；关闭时返回 nil。
func maybeNewModerationService(ctx context.Context, db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger) (moderationservice.Service, error) {
	if !cfg.Enabled {
		logger.Warn("content moderation is disabled; UGC writes use legacy business paths")
		return nil, nil
	}
	return newModerationService(ctx, db, cfg, logger)
}

// newModerationService 在任何普通内容写服务构造前完成规则加载，初始化失败时由启动层终止服务。
func newModerationService(ctx context.Context, db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger) (moderationservice.Service, error) {
	repo := moderationrepo.NewRepository(db)
	classifier, err := moderationservice.NewClassifierFromRepository(ctx, repo, logger)
	if err != nil {
		return nil, err
	}
	return moderationservice.NewService(
		repo, moderationservice.NewContentProcessor(), classifier,
		moderationservice.NewPolicyDecider(), cfg, logger, nil,
	), nil
}

// maybeNewModerationReviewService 在审核开启时组装管理端人工审核服务。
func maybeNewModerationReviewService(db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger) moderationservice.ReviewService {
	if !cfg.Enabled {
		return nil
	}
	return newModerationReviewService(db, cfg, logger)
}

// newModerationReviewService 组装管理端人工审核服务，复用同一数据库事实源。
func newModerationReviewService(db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger) moderationservice.ReviewService {
	return moderationservice.NewReviewService(
		moderationrepo.NewRepository(db), moderationservice.NewContentProcessor(), cfg, logger, nil,
	)
}
