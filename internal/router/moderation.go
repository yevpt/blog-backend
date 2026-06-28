package router

import (
	"context"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

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
