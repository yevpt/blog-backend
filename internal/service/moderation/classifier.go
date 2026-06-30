package moderation

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/vpt/blog-backend/internal/repository/moderationrule"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
	"go.uber.org/zap"
)

type classifier struct {
	logger   *zap.Logger
	snapshot atomic.Pointer[ruleindex.Snapshot]
}

// NewClassifier 创建分类器；nil 初始快照保持冷加载降级状态。
func NewClassifier(logger *zap.Logger, initial *ruleindex.Snapshot) Classifier {
	if logger == nil {
		logger = zap.NewNop()
	}
	result := &classifier{logger: logger}
	if initial != nil {
		if err := result.replaceSnapshot(initial); err != nil {
			logger.Warn("初始化文本审核规则失败，启用中风险降级", zap.Error(err))
		}
	}
	return result
}

// NewClassifierFromRepository 从当前已发布版本的规则行流构建运行时分类器。
func NewClassifierFromRepository(
	ctx context.Context,
	repo moderationrule.SnapshotRepository,
	limits ruleindex.Limits,
	logger *zap.Logger,
) (Classifier, error) {
	if repo == nil {
		return nil, errors.New("规则快照仓库不能为空")
	}
	current, err := repo.CurrentRuleset(ctx)
	if err != nil {
		return nil, err
	}
	source := func(ctx context.Context, visit func(ruleindex.SourceRule) error) error {
		return repo.StreamRules(ctx, current.ID, func(record moderationrule.RuleRecord) error {
			return visit(ruleindex.SourceRule{
				ID: record.ID, Type: record.RuleType, Pattern: record.Pattern,
				Risk: record.RiskLevel, Effect: record.Effect, Priority: record.Priority,
			})
		})
	}
	snapshot, _, err := ruleindex.Build(ctx, current.ID, source, limits)
	if err != nil {
		return nil, err
	}
	return NewClassifier(logger, snapshot), nil
}

func (c *classifier) Classify(processed ProcessedContent) Classification {
	snapshot := c.snapshot.Load()
	if snapshot == nil {
		return Classification{Risk: RiskMedium}
	}

	matched := snapshot.Match(textnorm.Normalize(processed.PlainText))
	return Classification{
		Risk:                 classificationRisk(matched.Risk),
		RuleMatchIDs:         matched.RuleIDs,
		RuleMatchesTruncated: matched.Truncated,
		RulesetVersion:       snapshot.Version(),
	}
}

func (c *classifier) ReplaceSnapshot(next *ruleindex.Snapshot) error {
	err := c.replaceSnapshot(next)
	if err != nil {
		fields := []zap.Field{zap.Error(err)}
		if next != nil {
			fields = append(fields, zap.Uint64("ruleset_version", next.Version()))
		}
		c.logger.Warn("文本审核规则替换失败，保留最后有效快照", fields...)
	}
	return err
}

func (c *classifier) replaceSnapshot(next *ruleindex.Snapshot) error {
	if next == nil || next.Version() == 0 || next.Stats().RuleCount == 0 {
		return errors.New("规则快照无效")
	}
	for {
		current := c.snapshot.Load()
		if current != nil && next.Version() <= current.Version() {
			return fmt.Errorf("ruleset version must increase: current=%d next=%d", current.Version(), next.Version())
		}
		if c.snapshot.CompareAndSwap(current, next) {
			return nil
		}
	}
}

func classificationRisk(risk ruleindex.Risk) RiskLevel {
	switch risk {
	case ruleindex.RiskLow:
		return RiskLow
	case ruleindex.RiskHigh:
		return RiskHigh
	default:
		return RiskMedium
	}
}
