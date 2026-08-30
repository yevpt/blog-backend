package app

import (
	"context"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/repository/moderationrule"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	rulemod "github.com/vpt/blog-backend/internal/service/moderationrule"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type moderationRuntime struct {
	service    moderationservice.Service
	ruleSvc    rulemod.Service
	ruleWorker rulemod.Worker
}

func maybeNewModerationService(
	ctx context.Context,
	db *gorm.DB,
	cfg config.ModerationConfig,
	logger *zap.Logger,
	store storage.ObjectStore,
) (moderationRuntime, error) {
	if !cfg.Enabled {
		logger.Warn("content moderation is disabled; UGC writes use legacy business paths")
		return moderationRuntime{}, nil
	}
	return newModerationService(ctx, db, cfg, logger, store)
}

func newModerationService(
	ctx context.Context,
	db *gorm.DB,
	cfg config.ModerationConfig,
	logger *zap.Logger,
	store storage.ObjectStore,
) (moderationRuntime, error) {
	repo := moderationrepo.NewRepository(db)
	ruleRepo := moderationrule.NewRepository(db)
	classifier, err := moderationservice.NewClassifierFromRepository(ctx, ruleRepo, moderationRuleIndexLimits(cfg.Rules), logger)
	if err != nil {
		return moderationRuntime{}, err
	}

	streamStore, _ := store.(rulemod.ImportObjectStore)
	ruleSvc := rulemod.NewManager(ruleRepo, streamStore, classifier, ruleManagerConfig(cfg.Rules), logger)
	var media moderationservice.MediaService
	var publisher moderationservice.ApprovedImagePublisher
	if readable, ok := store.(storage.ReadableObjectStore); ok {
		media = moderationmedia.NewService(readable, repo, cfg.Image, nil)
		publisher = moderationmedia.NewPublisher(readable, repo)
	}
	service := moderationservice.NewService(
		repo, moderationservice.NewContentProcessor(), classifier,
		moderationservice.NewPolicyDecider(), media, publisher, cfg, logger, nil,
	)
	return moderationRuntime{service: service, ruleSvc: ruleSvc, ruleWorker: rulemod.NewWorker(ruleSvc)}, nil
}

func ruleManagerConfig(cfg config.ModerationRulesConfig) rulemod.ManagerConfig {
	return rulemod.ManagerConfig{
		MaxPatternChars: cfg.MaxPatternChars, MaxKeywordRules: cfg.MaxKeywordRules,
		MaxEnabledRegexRules: cfg.MaxEnabledRegexRules, MaxImportRows: cfg.MaxImportRows,
		MaxImportFileMB: cfg.MaxImportFileMB, MaxRuleMatches: cfg.MaxRuleMatchesPerContent,
		MaxIndexMemoryMB: cfg.MaxIndexMemoryMB, MaxBuildPeakMemoryMB: cfg.MaxBuildPeakMemoryMB,
		IndexBuildTimeout: cfg.IndexBuildTimeout, CandidateCacheTTL: cfg.CandidateCacheTTL,
	}
}

func moderationRuleIndexLimits(cfg config.ModerationRulesConfig) ruleindex.Limits {
	return ruleindex.Limits{
		MaxKeywordRules: cfg.MaxKeywordRules, MaxRegexpRules: cfg.MaxEnabledRegexRules,
		MaxPatternRunes: cfg.MaxPatternChars, MaxMatchIDs: cfg.MaxRuleMatchesPerContent,
		MaxIndexMemoryBytes:     uint64(cfg.MaxIndexMemoryMB) * 1024 * 1024,
		MaxBuildPeakMemoryBytes: uint64(cfg.MaxBuildPeakMemoryMB) * 1024 * 1024,
	}
}

func maybeNewModerationReviewService(
	db *gorm.DB,
	cfg config.ModerationConfig,
	logger *zap.Logger,
	store storage.ObjectStore,
) moderationservice.ReviewService {
	if !cfg.Enabled {
		return nil
	}
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

func maybeNewModerationGovernanceService(
	db *gorm.DB,
	cfg config.ModerationConfig,
) moderationservice.GovernanceService {
	if !cfg.Enabled {
		return nil
	}
	return moderationservice.NewGovernanceService(moderationrepo.NewRepository(db), cfg.Governance, nil)
}

type moderationProfileReaderAdapter struct {
	governance moderationservice.GovernanceService
}

func (a *moderationProfileReaderAdapter) GetSanctionState(userID uint) (string, error) {
	profile, err := a.governance.GetProfile(context.Background(), uint64(userID))
	if err != nil {
		return "", err
	}
	return string(profile.SanctionState), nil
}

func newModerationProfileReader(governance moderationservice.GovernanceService) userservice.ModerationProfileReader {
	if governance == nil {
		return nil
	}
	return &moderationProfileReaderAdapter{governance: governance}
}

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
