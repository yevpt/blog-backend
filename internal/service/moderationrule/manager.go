package moderationrule

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"go.uber.org/zap"
)

// ManagerConfig 是规则管理服务的安全边界配置，由上层从 config 映射。
type ManagerConfig struct {
	MaxPatternChars      int
	MaxKeywordRules      int
	MaxEnabledRegexRules int
	MaxImportRows        int
	MaxImportFileMB      int
	MaxRuleMatches       int
	MaxIndexMemoryMB     int
	MaxBuildPeakMemoryMB int
	IndexBuildTimeout    time.Duration
	CandidateCacheTTL    time.Duration
	PollInterval         time.Duration
}

// manager 同时实现 Service 和 Worker，持有唯一的候选缓存和当前快照引用。
type manager struct {
	repo     repoMod.ManagementRepository
	store    ImportObjectStore
	replacer SnapshotReplacer
	limits   ruleindex.Limits
	cfg      ManagerConfig
	logger   *zap.Logger
	cache    *candidateCache

	// publishMu 保证同一时刻最多一个候选进入发布。
	publishMu chan struct{}

	// currentSnapshot 缓存当前已发布快照供试跑复用。
	currentSnapshot atomic.Pointer[ruleindex.Snapshot]

	pollInterval time.Duration
}

// NewManager 创建规则管理服务，所有依赖由调用方注入。
func NewManager(
	repo repoMod.ManagementRepository,
	store ImportObjectStore,
	replacer SnapshotReplacer,
	cfg ManagerConfig,
	logger *zap.Logger,
) Service {
	return newManager(repo, store, replacer, cfg, logger)
}

// NewWorker 从 Service 创建后台 worker。两者必须共享同一实例。
func NewWorker(svc Service) Worker {
	if m, ok := svc.(*manager); ok {
		return m
	}
	return nil
}

func newManager(
	repo repoMod.ManagementRepository,
	store ImportObjectStore,
	replacer SnapshotReplacer,
	cfg ManagerConfig,
	logger *zap.Logger,
) *manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	normalizeManagerConfig(&cfg)
	return &manager{
		repo:         repo,
		store:        store,
		replacer:     replacer,
		limits:       managerLimits(cfg),
		cfg:          cfg,
		logger:       logger,
		cache:        newCandidateCache(cfg.CandidateCacheTTL, time.Now),
		publishMu:    make(chan struct{}, 1),
		pollInterval: cfg.PollInterval,
	}
}

// Classifier 返回快照替换器，供运行时共享。
func (m *manager) Classifier() SnapshotReplacer {
	return m.replacer
}

// ListRules 透传游标查询到仓储层。
func (m *manager) ListRules(ctx context.Context, query ListQuery) (repoMod.RulePage, error) {
	return m.repo.ListRules(ctx, repoMod.RuleFilter{
		AfterID:       query.AfterID,
		ExactID:       query.ExactID,
		Limit:         query.Limit,
		ExactPattern:  query.ExactPattern,
		PatternPrefix: query.PatternPrefix,
		Category:      query.Category,
		RuleType:      query.RuleType,
		RiskLevel:     query.RiskLevel,
		Effect:        query.Effect,
		SourceID:      query.SourceID,
		Active:        query.Active,
	})
}

// Metadata 返回受控分类目录和来源列表。
func (m *manager) Metadata(ctx context.Context) (Metadata, error) {
	sources, err := m.repo.ListSources(ctx)
	if err != nil {
		return Metadata{}, fmt.Errorf("读取规则来源目录: %w", err)
	}
	sourceEntries := make([]SourceEntry, 0, len(sources))
	for _, s := range sources {
		sourceEntries = append(sourceEntries, SourceEntry{ID: s.ID, Name: s.Name})
	}
	return Metadata{
		Categories: moderationCategories(),
		RiskLevels: []string{"low", "medium", "high"},
		Effects:    []string{"review", "allow"},
		RuleTypes:  []string{"keyword", "regexp", "composite"},
		Sources:    sourceEntries,
	}, nil
}

// Status 返回当前规则集和候选状态摘要。
func (m *manager) Status(ctx context.Context) (Status, error) {
	record, err := m.repo.CurrentStatus(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("读取规则集状态: %w", err)
	}
	status := Status{
		CurrentRulesetID: record.CurrentRulesetID,
		RuleCount:        record.RuleCount,
		KeywordCount:     record.KeywordCount,
		RegexpCount:      record.RegexpCount,
		CompositeCount:   record.CompositeCount,
		IndexBytes:       record.IndexBytes,
		BuildPeakBytes:   record.BuildPeakBytes,
		BuildDurationMS:  record.BuildDurationMS,
		IndexObjectKey:   record.IndexObjectKey,
		IndexSHA256:      record.IndexSHA256,
		UpdatedAt:        record.UpdatedAt,
	}
	if record.Candidate != nil {
		status.Candidate = &CandidateStatus{
			RulesetID:      record.Candidate.RulesetID,
			Status:         record.Candidate.Status,
			BaseRulesetID:  record.Candidate.BaseRulesetID,
			RuleCount:      record.Candidate.RuleCount,
			KeywordCount:   record.Candidate.KeywordCount,
			RegexpCount:    record.Candidate.RegexpCount,
			CompositeCount: record.Candidate.CompositeCount,
			IndexBytes:     record.Candidate.IndexBytes,
			BuildPeakBytes: record.Candidate.BuildPeakBytes,
			FailureCode:    record.Candidate.FailureCode,
			CreatedAt:      record.Candidate.CreatedAt,
			UpdatedAt:      record.Candidate.UpdatedAt,
		}
	}
	return status, nil
}

func normalizeManagerConfig(cfg *ManagerConfig) {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.CandidateCacheTTL <= 0 {
		cfg.CandidateCacheTTL = 10 * time.Minute
	}
	if cfg.MaxImportFileMB <= 0 {
		cfg.MaxImportFileMB = 50
	}
}

func managerLimits(cfg ManagerConfig) ruleindex.Limits {
	return ruleindex.Limits{
		MaxKeywordRules:         cfg.MaxKeywordRules,
		MaxRegexpRules:          cfg.MaxEnabledRegexRules,
		MaxPatternRunes:         cfg.MaxPatternChars,
		MaxMatchIDs:             cfg.MaxRuleMatches,
		MaxIndexMemoryBytes:     uint64(cfg.MaxIndexMemoryMB) * 1024 * 1024,
		MaxBuildPeakMemoryBytes: uint64(cfg.MaxBuildPeakMemoryMB) * 1024 * 1024,
	}
}

func moderationCategories() []CategoryEntry {
	return []CategoryEntry{
		{Key: "politics", Name: "政治"},
		{Key: "pornography", Name: "色情"},
		{Key: "violence", Name: "暴力"},
		{Key: "terrorism", Name: "恐怖主义"},
		{Key: "gambling", Name: "赌博"},
		{Key: "drugs", Name: "毒品"},
		{Key: "fraud", Name: "欺诈"},
		{Key: "abuse", Name: "辱骂"},
		{Key: "advertising", Name: "广告"},
		{Key: "minors", Name: "未成年人"},
		{Key: "other", Name: "其他"},
	}
}
