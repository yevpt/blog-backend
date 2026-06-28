package moderation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"
)

type classifier struct {
	logger   *zap.Logger
	snapshot atomic.Pointer[compiledSnapshot]
}

type compiledSnapshot struct {
	version uint64
	rules   []runtimeRule
}

type runtimeRule struct {
	id      uint64
	risk    RiskLevel
	keyword string
	regexp  *regexp.Regexp
	signals []string
}

// NewClassifier 创建分类器；初始快照无效时保持冷加载降级状态。
func NewClassifier(logger *zap.Logger, initial RuleSnapshot) Classifier {
	if logger == nil {
		logger = zap.NewNop()
	}
	result := &classifier{logger: logger}
	if err := result.replaceSnapshot(initial); err != nil {
		logger.Warn("初始化文本审核规则失败，启用中风险降级", zap.Error(err))
	}
	return result
}

func (c *classifier) Classify(processed ProcessedContent) Classification {
	snapshot := c.snapshot.Load()
	if snapshot == nil {
		return Classification{Risk: RiskMedium}
	}

	normalized := NormalizeText(processed.PlainText)
	result := Classification{Risk: RiskLow, RuleMatchIDs: make([]uint64, 0), RulesetVersion: snapshot.version}
	for _, rule := range snapshot.rules {
		if !rule.matches(normalized) {
			continue
		}
		result.RuleMatchIDs = append(result.RuleMatchIDs, rule.id)
		if riskRank(rule.risk) > riskRank(result.Risk) {
			result.Risk = rule.risk
		}
	}
	return result
}

func (c *classifier) ReplaceSnapshot(next RuleSnapshot) error {
	err := c.replaceSnapshot(next)
	if err != nil {
		c.logger.Warn("文本审核规则替换失败，保留最后有效快照",
			zap.Uint64("ruleset_version", next.Version),
			zap.Error(err),
		)
	}
	return err
}

func (c *classifier) replaceSnapshot(next RuleSnapshot) error {
	compiled, err := compileSnapshot(next)
	if err != nil {
		return err
	}

	for {
		current := c.snapshot.Load()
		if current != nil && next.Version <= current.version {
			return fmt.Errorf("ruleset version must increase: current=%d next=%d", current.version, next.Version)
		}
		if c.snapshot.CompareAndSwap(current, compiled) {
			return nil
		}
	}
}

func compileSnapshot(snapshot RuleSnapshot) (*compiledSnapshot, error) {
	if len(snapshot.Rules) == 0 {
		return nil, ErrEmptyRuleset
	}

	rules := make([]runtimeRule, 0, len(snapshot.Rules))
	for _, source := range snapshot.Rules {
		compiled, err := compileRule(source)
		if err != nil {
			return nil, fmt.Errorf("compile moderation rule %d: %w", source.ID, err)
		}
		rules = append(rules, compiled)
	}
	return &compiledSnapshot{version: snapshot.Version, rules: rules}, nil
}

func compileRule(source CompiledRule) (runtimeRule, error) {
	if source.ID == 0 {
		return runtimeRule{}, errors.New("rule ID must be positive")
	}
	if riskRank(source.Risk) == 0 {
		return runtimeRule{}, fmt.Errorf("invalid risk level %q", source.Risk)
	}

	rule := runtimeRule{id: source.ID, risk: source.Risk}
	switch source.Type {
	case RuleKeyword:
		rule.keyword = NormalizeText(source.Pattern)
		if rule.keyword == "" {
			return runtimeRule{}, errors.New("keyword cannot be empty")
		}
	case RuleRegexp:
		compiled, err := compileNormalizedRegexp(source.Pattern)
		if err != nil {
			return runtimeRule{}, fmt.Errorf("invalid RE2 pattern: %w", err)
		}
		rule.regexp = compiled
	case RuleComposite:
		signals, err := compileCompositeSignals(source.Pattern)
		if err != nil {
			return runtimeRule{}, err
		}
		rule.signals = signals
	default:
		return runtimeRule{}, fmt.Errorf("invalid rule type %q", source.Type)
	}
	return rule, nil
}

func compileCompositeSignals(pattern string) ([]string, error) {
	parts := strings.Split(pattern, "&&")
	if len(parts) < 2 {
		return nil, errors.New("composite rule requires at least two && signals")
	}

	signals := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := NormalizeText(part)
		if normalized == "" {
			return nil, errors.New("composite signal cannot be empty")
		}
		signals = append(signals, normalized)
	}
	return signals, nil
}

func (r runtimeRule) matches(text string) bool {
	if r.keyword != "" {
		return strings.Contains(text, r.keyword)
	}
	if r.regexp != nil {
		return r.regexp.MatchString(text)
	}
	for _, signal := range r.signals {
		if !strings.Contains(text, signal) {
			return false
		}
	}
	return len(r.signals) > 0
}

func riskRank(risk RiskLevel) int {
	switch risk {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	default:
		return 0
	}
}
