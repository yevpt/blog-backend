package router

import (
	"context"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/repository/moderationrule"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// maybeNewModerationService 在审核开启时加载规则并构造运行时；关闭时返回 nil。
func maybeNewModerationService(ctx context.Context, db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger, store storage.ObjectStore) (moderationservice.Service, error) {
	if !cfg.Enabled {
		logger.Warn("content moderation is disabled; UGC writes use legacy business paths")
		return nil, nil
	}
	return newModerationService(ctx, db, cfg, logger, store)
}

// newModerationService 在任何普通内容写服务构造前完成规则加载，初始化失败时由启动层终止服务。
func newModerationService(ctx context.Context, db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger, store storage.ObjectStore) (moderationservice.Service, error) {
	repo := moderationrepo.NewRepository(db)
	snapshotRepo := moderationrule.NewRepository(db)
	classifier, err := moderationservice.NewClassifierFromRepository(ctx, snapshotRepo, moderationRuleIndexLimits(cfg.Rules), logger)
	if err != nil {
		return nil, err
	}
	var media moderationservice.MediaService
	if readable, ok := store.(storage.ReadableObjectStore); ok {
		media = moderationmedia.NewService(readable, repo, cfg.Image, nil)
	}
	return moderationservice.NewService(
		repo, moderationservice.NewContentProcessor(), classifier,
		moderationservice.NewPolicyDecider(), media, cfg, logger, nil,
	), nil
}

func moderationRuleIndexLimits(cfg config.ModerationRulesConfig) ruleindex.Limits {
	return ruleindex.Limits{
		MaxKeywordRules:         cfg.MaxKeywordRules,
		MaxRegexpRules:          cfg.MaxEnabledRegexRules,
		MaxPatternRunes:         cfg.MaxPatternChars,
		MaxMatchIDs:             cfg.MaxRuleMatchesPerContent,
		MaxIndexMemoryBytes:     uint64(cfg.MaxIndexMemoryMB) * 1024 * 1024,
		MaxBuildPeakMemoryBytes: uint64(cfg.MaxBuildPeakMemoryMB) * 1024 * 1024,
	}
}

// maybeNewModerationReviewService 在审核开启时组装管理端人工审核服务。
func maybeNewModerationReviewService(db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger, store storage.ObjectStore) moderationservice.ReviewService {
	if !cfg.Enabled {
		return nil
	}
	return newModerationReviewService(db, cfg, logger, store)
}

// maybeNewModerationGovernanceService 在审核开启时创建用户画像治理服务。
func maybeNewModerationGovernanceService(db *gorm.DB, cfg config.ModerationConfig) moderationservice.GovernanceService {
	if !cfg.Enabled {
		return nil
	}
	return moderationservice.NewGovernanceService(moderationrepo.NewRepository(db), cfg.Governance, nil)
}

// maybeNewModerationOperationsService 在审核开启时组装全站治理和紧急处置服务。
func maybeNewModerationOperationsService(
	db *gorm.DB,
	cfg config.ModerationConfig,
	governance moderationservice.GovernanceService,
) moderationservice.OperationsService {
	if !cfg.Enabled || governance == nil {
		return nil
	}
	return moderationservice.NewOperationsService(moderationrepo.NewRepository(db), governance, cfg, nil)
}

// newModerationReviewService 组装管理端人工审核服务，复用同一数据库事实源。

func newModerationReviewService(db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger, store storage.ObjectStore) moderationservice.ReviewService {
	repo := moderationrepo.NewRepository(db)
	var cleaner moderationservice.PreviewCleaner
	if readable, ok := store.(storage.ReadableObjectStore); ok {
		cleaner = moderationmedia.NewService(readable, repo, cfg.Image, nil)
	}
	return moderationservice.NewReviewService(
		repo, moderationservice.NewContentProcessor(), cleaner, cfg, logger, nil,
	)
}
