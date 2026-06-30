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

type repository struct {
	db *gorm.DB
}

// NewRepository 创建规则快照仓库，数据库由调用方注入。
func NewRepository(db *gorm.DB) SnapshotRepository {
	return &repository{db: db}
}
