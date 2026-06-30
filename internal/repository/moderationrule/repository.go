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

// ManagementRepository 在快照能力之上提供管理端游标查询、来源目录、重复摘要批查和状态摘要。
type ManagementRepository interface {
	SnapshotRepository
	ListRules(ctx context.Context, filter RuleFilter) (RulePage, error)
	ListSources(ctx context.Context) ([]SourceRecord, error)
	EnsureSource(ctx context.Context, name string) (SourceRecord, error)
	FindDuplicateHashes(ctx context.Context, currentRulesetID uint64, hashes []DedupeHash) (map[DedupeHash]uint64, error)
	CurrentStatus(ctx context.Context) (StatusRecord, error)
}

type repository struct {
	db *gorm.DB
}

// NewRepository 创建规则快照仓库，数据库由调用方注入。
func NewRepository(db *gorm.DB) ManagementRepository {
	return &repository{db: db}
}
