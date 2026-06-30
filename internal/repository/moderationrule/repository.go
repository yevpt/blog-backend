package moderationrule

import (
	"context"

	"gorm.io/gorm"
)

// SnapshotRepository 提供启动期当前规则集和规则行流。
type SnapshotRepository interface {
	CurrentRuleset(ctx context.Context) (RulesetRecord, error)
	StreamRules(ctx context.Context, version uint64, visit func(RuleRecord) error) error
}

// ManagementRepository 在快照能力之上提供管理端游标查询、来源目录、重复摘要批查、候选创建发布和状态摘要。
type ManagementRepository interface {
	SnapshotRepository
	ListRules(ctx context.Context, filter RuleFilter) (RulePage, error)
	ListSources(ctx context.Context) ([]SourceRecord, error)
	EnsureSource(ctx context.Context, name string) (SourceRecord, error)
	FindDuplicateHashes(ctx context.Context, currentRulesetID uint64, hashes []DedupeHash) (map[DedupeHash]uint64, error)
	CurrentStatus(ctx context.Context) (StatusRecord, error)
	CreateCandidate(ctx context.Context, cmd CreateCandidateCommand) (CandidateRecord, error)
	PublishCandidate(ctx context.Context, id, expectedBase uint64) error
	ClaimNextRuleset(ctx context.Context, status string) (*CandidateRecord, error)
	FailRuleset(ctx context.Context, id uint64, failureCode string) error
	SaveRulesetBuildResult(ctx context.Context, id uint64, result BuildResult) error
	CancelCandidate(ctx context.Context, id, actorID uint64) error
	StreamCandidateRules(ctx context.Context, baseVersion, candidateID uint64, visit func(RuleRecord) error) error
	GetRulesetRemovals(ctx context.Context, rulesetID uint64) ([]uint64, error)
	GetCandidate(ctx context.Context, id uint64) (CandidateRecord, error)
	HasImportForRuleset(ctx context.Context, rulesetID uint64) (bool, error)
	GetRulesByIDs(ctx context.Context, ids []uint64) ([]RuleListRecord, error)
}

type repository struct {
	db *gorm.DB
}

// NewRepository 创建规则快照仓库，数据库由调用方注入。
func NewRepository(db *gorm.DB) ManagementRepository {
	return &repository{db: db}
}
