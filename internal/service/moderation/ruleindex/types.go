package ruleindex

import (
	"context"
	"errors"
	"regexp"
)

const (
	defaultMaxKeywordRules = 500000
	defaultMaxRegexpRules  = 200
	defaultMaxPatternRunes = 500
	defaultMaxMatchIDs     = 128
	hardMaxRegexpRules     = 500
	defaultMaxIndexBytes   = 512 * 1024 * 1024
	defaultMaxPeakBytes    = 1024 * 1024 * 1024
)

var (
	// ErrIndexLimit 表示规则数量、模式长度或内存预算超过安全边界。
	ErrIndexLimit = errors.New("规则索引超过安全边界")
	// ErrEmptyRuleset 表示规则源没有可构建的规则。
	ErrEmptyRuleset = errors.New("规则集不能为空")
	// ErrIndexCorrupt 表示索引文件格式、结构或校验和无效。
	ErrIndexCorrupt = errors.New("规则索引文件损坏")
)

// Risk 是索引内部使用的紧凑风险等级。
type Risk uint8

const (
	RiskLow Risk = iota + 1
	RiskMedium
	RiskHigh
)

// Effect 是关键词规则的匹配效果。
type Effect uint8

const (
	EffectReview Effect = iota + 1
	EffectAllow
)

type ruleType uint8

const (
	ruleTypeKeyword ruleType = iota + 1
	ruleTypeRegexp
	ruleTypeComposite
)

// SourceRule 是构建器逐条接收的最小规则事实。
type SourceRule struct {
	ID       uint64
	Type     string
	Pattern  string
	Risk     string
	Effect   string
	Priority int32
}

// Source 以回调方式流式提供规则，避免仓库预先构造完整切片。
type Source func(context.Context, func(SourceRule) error) error

// Limits 定义构建和运行时的硬边界；零值使用安全默认值。
type Limits struct {
	MaxKeywordRules         int
	MaxRegexpRules          int
	MaxPatternRunes         int
	MaxMatchIDs             int
	MaxIndexMemoryBytes     uint64
	MaxBuildPeakMemoryBytes uint64
	CurrentIndexMemoryBytes uint64
}

// Stats 描述紧凑快照的确定性结构与容量统计。
type Stats struct {
	RuleCount         int
	KeywordCount      int
	RegexpCount       int
	CompositeCount    int
	StateCount        int
	EdgeCount         int
	DirectOutputCount int
	IndexBytes        uint64
	BuildPeakBytes    uint64
}

// MatchResult 返回最终风险和有界、确定性排序的规则 ID。
type MatchResult struct {
	Risk          Risk
	RuleIDs       []uint64
	SuppressedIDs []uint64
	Truncated     bool
}

type state struct {
	edgeStart    uint32
	edgeCount    uint32
	failure      uint32
	suffix       uint32
	outputStart  uint32
	outputCount  uint32
	longestAllow uint16
}

type edge struct {
	label  rune
	target uint32
}

type nonKeywordRule struct {
	ruleIndex uint32
	kind      ruleType
	pattern   string
	regexp    *regexp.Regexp
	signals   []string
}

// Snapshot 是构建完成后只读的紧凑规则索引。
type Snapshot struct {
	version     uint64
	states      []state
	edges       []edge
	outputs     []uint32
	ruleIDs     []uint64
	risks       []Risk
	effects     []Effect
	priorities  []int32
	lengths     []uint16
	regexps     []nonKeywordRule
	stats       Stats
	maxMatchIDs int
}

// Version 返回快照对应的规则集版本。
func (s *Snapshot) Version() uint64 {
	if s == nil {
		return 0
	}
	return s.version
}

// Stats 返回构建时固化的结构统计。
func (s *Snapshot) Stats() Stats {
	if s == nil {
		return Stats{}
	}
	return s.stats
}
