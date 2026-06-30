package router

import (
	"context"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/repository/moderationrule"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	rulemod "github.com/vpt/blog-backend/internal/service/moderationrule"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// moderationRuntime 收集审核开启时由路由层构造的共享实例。
type moderationRuntime struct {
	service    moderationservice.Service
	ruleSvc    rulemod.Service
	ruleWorker rulemod.Worker
}

// maybeNewModerationService 在审核开启时加载规则并构造运行时；关闭时返回空值。
func maybeNewModerationService(ctx context.Context, db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger, store storage.ObjectStore) (moderationRuntime, error) {
	if !cfg.Enabled {
		logger.Warn("content moderation is disabled; UGC writes use legacy business paths")
		return moderationRuntime{}, nil
	}
	return newModerationService(ctx, db, cfg, logger, store)
}

// newModerationService 在任何普通内容写服务构造前完成规则加载，初始化失败时由启动层终止服务。
// 关键约束：分类器实例同时注入核心审核服务和规则管理服务，保证发布时原子替换同一快照。
func newModerationService(ctx context.Context, db *gorm.DB, cfg config.ModerationConfig, logger *zap.Logger, store storage.ObjectStore) (moderationRuntime, error) {
	repo := moderationrepo.NewRepository(db)
	ruleRepo := moderationrule.NewRepository(db)
	classifier, err := moderationservice.NewClassifierFromRepository(ctx, ruleRepo, moderationRuleIndexLimits(cfg.Rules), logger)
	if err != nil {
		return moderationRuntime{}, err
	}

	// 构造规则管理服务，与核心审核共享同一分类器。
	streamStore, _ := store.(rulemod.ImportObjectStore)
	ruleSvc := rulemod.NewManager(
		ruleRepo,
		streamStore,
		classifier,
		ruleManagerConfig(cfg.Rules),
		logger,
	)

	var media moderationservice.MediaService
	var publisher moderationservice.ApprovedImagePublisher
	if readable, ok := store.(storage.ReadableObjectStore); ok {
		media = moderationmedia.NewService(readable, repo, cfg.Image, nil)
		publisher = moderationmedia.NewPublisher(readable, repo)
	}
	svc := moderationservice.NewService(
		repo, moderationservice.NewContentProcessor(), classifier,
		moderationservice.NewPolicyDecider(), media, publisher, cfg, logger, nil,
	)

	return moderationRuntime{
		service:    svc,
		ruleSvc:    ruleSvc,
		ruleWorker: rulemod.NewWorker(ruleSvc),
	}, nil
}

func ruleManagerConfig(cfg config.ModerationRulesConfig) rulemod.ManagerConfig {
	return rulemod.ManagerConfig{
		MaxPatternChars:      cfg.MaxPatternChars,
		MaxKeywordRules:      cfg.MaxKeywordRules,
		MaxEnabledRegexRules: cfg.MaxEnabledRegexRules,
		MaxImportRows:        cfg.MaxImportRows,
		MaxImportFileMB:      cfg.MaxImportFileMB,
		MaxRuleMatches:       cfg.MaxRuleMatchesPerContent,
		MaxIndexMemoryMB:     cfg.MaxIndexMemoryMB,
		MaxBuildPeakMemoryMB: cfg.MaxBuildPeakMemoryMB,
		IndexBuildTimeout:    cfg.IndexBuildTimeout,
		CandidateCacheTTL:    cfg.CandidateCacheTTL,
	}
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
	var publisher moderationservice.ApprovedImagePublisher
	if readable, ok := store.(storage.ReadableObjectStore); ok {
		cleaner = moderationmedia.NewService(readable, repo, cfg.Image, nil)
		publisher = moderationmedia.NewPublisher(readable, repo)
	}
	return moderationservice.NewReviewService(
		repo, moderationservice.NewContentProcessor(), cleaner, publisher, cfg, logger, nil,
	)
}
