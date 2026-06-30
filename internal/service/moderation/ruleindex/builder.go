package ruleindex

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

const noLink = math.MaxUint32

type transitionKey struct {
	state uint32
	label rune
}

type buildState struct {
	firstEdge          uint32
	firstOutput        uint32
	directLongestAllow uint16
}

type buildEdge struct {
	label  rune
	target uint32
	next   uint32
}

type buildOutput struct {
	rule uint32
	next uint32
}

type builder struct {
	limits      Limits
	states      []buildState
	edges       []buildEdge
	outputs     []buildOutput
	transitions map[transitionKey]uint32
	seenIDs     map[uint64]struct{}
	ruleIDs     []uint64
	risks       []Risk
	effects     []Effect
	priorities  []int32
	lengths     []uint16
	regexps     []nonKeywordRule
	keywords    int
	regexpCount int
	composites  int
}

// Build 从规则行流构建不可变紧凑快照。
func Build(ctx context.Context, version uint64, source Source, limits Limits) (*Snapshot, Stats, error) {
	if source == nil {
		return nil, Stats{}, errorsForRule(0, "规则源不能为空")
	}
	limits, err := normalizeLimits(limits)
	if err != nil {
		return nil, Stats{}, err
	}

	b := newBuilder(limits)
	err = source(ctx, func(rule SourceRule) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return b.addRule(rule)
	})
	if err != nil {
		return nil, Stats{}, err
	}
	if len(b.ruleIDs) == 0 {
		return nil, Stats{}, ErrEmptyRuleset
	}
	return b.compact(ctx, version)
}

func normalizeLimits(limits Limits) (Limits, error) {
	if limits.MaxKeywordRules == 0 {
		limits.MaxKeywordRules = defaultMaxKeywordRules
	}
	if limits.MaxRegexpRules == 0 {
		limits.MaxRegexpRules = defaultMaxRegexpRules
	}
	if limits.MaxPatternRunes == 0 {
		limits.MaxPatternRunes = defaultMaxPatternRunes
	}
	if limits.MaxMatchIDs == 0 {
		limits.MaxMatchIDs = defaultMaxMatchIDs
	}
	if limits.MaxKeywordRules < 0 || limits.MaxRegexpRules < 0 || limits.MaxPatternRunes < 0 || limits.MaxMatchIDs < 0 {
		return Limits{}, fmt.Errorf("%w: 配置必须为正数", ErrIndexLimit)
	}
	if limits.MaxPatternRunes > math.MaxUint16 {
		return Limits{}, fmt.Errorf("%w: 模式长度超过 uint16", ErrIndexLimit)
	}
	if limits.MaxKeywordRules > defaultMaxKeywordRules {
		return Limits{}, fmt.Errorf("%w: 关键词规则上限不得超过 %d", ErrIndexLimit, defaultMaxKeywordRules)
	}
	if limits.MaxRegexpRules > hardMaxRegexpRules {
		return Limits{}, fmt.Errorf("%w: 非关键词规则上限不得超过 %d", ErrIndexLimit, hardMaxRegexpRules)
	}
	if limits.MaxMatchIDs > defaultMaxMatchIDs {
		return Limits{}, fmt.Errorf("%w: 命中 ID 上限不得超过 %d", ErrIndexLimit, defaultMaxMatchIDs)
	}
	return limits, nil
}

func newBuilder(limits Limits) *builder {
	return &builder{
		limits:      limits,
		states:      []buildState{{firstEdge: noLink, firstOutput: noLink}},
		transitions: make(map[transitionKey]uint32),
		seenIDs:     make(map[uint64]struct{}),
	}
}

func (b *builder) addRule(source SourceRule) error {
	if source.ID == 0 {
		return errorsForRule(source.ID, "规则 ID 必须为正数")
	}
	if _, exists := b.seenIDs[source.ID]; exists {
		return errorsForRule(source.ID, "规则 ID 重复")
	}
	if !utf8.ValidString(source.Pattern) {
		return errorsForRule(source.ID, "模式必须是 UTF-8")
	}
	if utf8.RuneCountInString(source.Pattern) > b.limits.MaxPatternRunes {
		return limitForRule(source.ID, "原始模式超过长度上限")
	}

	risk, err := parseRisk(source.Risk)
	if err != nil {
		return errorsForRule(source.ID, err.Error())
	}
	effect, err := parseEffect(source.Effect)
	if err != nil {
		return errorsForRule(source.ID, err.Error())
	}
	kind, err := parseRuleType(source.Type)
	if err != nil {
		return errorsForRule(source.ID, err.Error())
	}
	if effect == EffectAllow && kind != ruleTypeKeyword {
		return errorsForRule(source.ID, "allow 仅支持关键词规则")
	}

	prepared, patternRunes, err := b.preparePattern(source.ID, kind, source.Pattern)
	if err != nil {
		return err
	}
	if len(b.ruleIDs) >= math.MaxUint32 {
		return fmt.Errorf("%w: 规则序号超过 uint32", ErrIndexLimit)
	}

	ruleIndex := uint32(len(b.ruleIDs))
	b.seenIDs[source.ID] = struct{}{}
	b.ruleIDs = append(b.ruleIDs, source.ID)
	b.risks = append(b.risks, risk)
	b.effects = append(b.effects, effect)
	b.priorities = append(b.priorities, source.Priority)
	b.lengths = append(b.lengths, uint16(patternRunes))

	switch kind {
	case ruleTypeKeyword:
		b.addKeyword([]rune(prepared.pattern), ruleIndex, effect)
		b.keywords++
	case ruleTypeRegexp:
		prepared.ruleIndex = ruleIndex
		b.regexps = append(b.regexps, prepared)
		b.regexpCount++
	case ruleTypeComposite:
		prepared.ruleIndex = ruleIndex
		b.regexps = append(b.regexps, prepared)
		b.composites++
	}
	return nil
}

func (b *builder) preparePattern(id uint64, kind ruleType, pattern string) (nonKeywordRule, int, error) {
	switch kind {
	case ruleTypeKeyword:
		if b.keywords >= b.limits.MaxKeywordRules {
			return nonKeywordRule{}, 0, fmt.Errorf("%w: 关键词规则超过 %d", ErrIndexLimit, b.limits.MaxKeywordRules)
		}
		normalized := textnorm.Normalize(pattern)
		length := utf8.RuneCountInString(normalized)
		if length == 0 {
			return nonKeywordRule{}, 0, errorsForRule(id, "关键词归一化后不能为空")
		}
		if length > b.limits.MaxPatternRunes {
			return nonKeywordRule{}, 0, limitForRule(id, "归一化模式超过长度上限")
		}
		return nonKeywordRule{kind: kind, pattern: normalized}, length, nil
	case ruleTypeRegexp:
		if b.regexpCount+b.composites >= b.limits.MaxRegexpRules {
			return nonKeywordRule{}, 0, fmt.Errorf("%w: 非关键词规则超过 %d", ErrIndexLimit, b.limits.MaxRegexpRules)
		}
		compiled, err := textnorm.CompileRegexp(pattern)
		if err != nil {
			return nonKeywordRule{}, 0, errorsForRule(id, "正则模式无效: "+err.Error())
		}
		return nonKeywordRule{kind: kind, pattern: pattern, regexp: compiled}, 0, nil
	case ruleTypeComposite:
		if b.regexpCount+b.composites >= b.limits.MaxRegexpRules {
			return nonKeywordRule{}, 0, fmt.Errorf("%w: 非关键词规则超过 %d", ErrIndexLimit, b.limits.MaxRegexpRules)
		}
		signals, err := compileComposite(pattern, b.limits.MaxPatternRunes)
		if err != nil {
			return nonKeywordRule{}, 0, fmt.Errorf("构建审核规则 %d: %w", id, err)
		}
		return nonKeywordRule{kind: kind, pattern: pattern, signals: signals}, 0, nil
	default:
		return nonKeywordRule{}, 0, errorsForRule(id, "规则类型无效")
	}
}

func (b *builder) addKeyword(pattern []rune, ruleIndex uint32, effect Effect) {
	current := uint32(0)
	for _, label := range pattern {
		key := transitionKey{state: current, label: label}
		next, exists := b.transitions[key]
		if !exists {
			next = uint32(len(b.states))
			b.states = append(b.states, buildState{firstEdge: noLink, firstOutput: noLink})
			edgeIndex := uint32(len(b.edges))
			b.edges = append(b.edges, buildEdge{label: label, target: next, next: b.states[current].firstEdge})
			b.states[current].firstEdge = edgeIndex
			b.transitions[key] = next
		}
		current = next
	}

	outputIndex := uint32(len(b.outputs))
	b.outputs = append(b.outputs, buildOutput{rule: ruleIndex, next: b.states[current].firstOutput})
	b.states[current].firstOutput = outputIndex
	if effect == EffectAllow && b.lengths[ruleIndex] > b.states[current].directLongestAllow {
		b.states[current].directLongestAllow = b.lengths[ruleIndex]
	}
}

func compileComposite(pattern string, maxRunes int) ([]string, error) {
	parts := strings.Split(pattern, "&&")
	if len(parts) < 2 {
		return nil, fmt.Errorf("组合规则至少需要两个 && 信号")
	}
	signals := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := textnorm.Normalize(part)
		length := utf8.RuneCountInString(normalized)
		if length == 0 {
			return nil, fmt.Errorf("组合信号归一化后不能为空")
		}
		if length > maxRunes {
			return nil, fmt.Errorf("%w: 组合信号超过长度上限", ErrIndexLimit)
		}
		signals = append(signals, normalized)
	}
	return signals, nil
}

func parseRisk(value string) (Risk, error) {
	switch value {
	case "low":
		return RiskLow, nil
	case "medium":
		return RiskMedium, nil
	case "high":
		return RiskHigh, nil
	default:
		return 0, fmt.Errorf("风险等级无效: %q", value)
	}
}

func parseEffect(value string) (Effect, error) {
	switch value {
	case "review":
		return EffectReview, nil
	case "allow":
		return EffectAllow, nil
	default:
		return 0, fmt.Errorf("规则效果无效: %q", value)
	}
}

func parseRuleType(value string) (ruleType, error) {
	switch value {
	case "keyword":
		return ruleTypeKeyword, nil
	case "regexp":
		return ruleTypeRegexp, nil
	case "composite":
		return ruleTypeComposite, nil
	default:
		return 0, fmt.Errorf("规则类型无效: %q", value)
	}
}

func errorsForRule(id uint64, message string) error {
	if id == 0 {
		return fmt.Errorf("构建审核规则: %s", message)
	}
	return fmt.Errorf("构建审核规则 %d: %s", id, message)
}

func limitForRule(id uint64, message string) error {
	return fmt.Errorf("%w: 规则 %d %s", ErrIndexLimit, id, message)
}
