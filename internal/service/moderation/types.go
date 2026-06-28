package moderation

// RiskLevel 表示文本规则判定出的最高风险等级。
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// RuleType 表示规则的匹配方式。
type RuleType string

const (
	RuleKeyword   RuleType = "keyword"
	RuleRegexp    RuleType = "regexp"
	RuleComposite RuleType = "composite"
)

// ProcessedContent 同时保存安全展示正文和分类所需的纯文本。
type ProcessedContent struct {
	// Published 是安全清洗后可用于展示的正文，保留原始简繁表达。
	Published string
	// PlainText 是从 Published 提取、供分类器继续归一化的纯文本。
	PlainText string
	// Links 是清洗后仍被允许的链接。
	Links []string
}

// ContentProcessor 在正文进入发布和分类链路前执行统一安全处理。
type ContentProcessor interface {
	Process(raw string, limit int) (ProcessedContent, error)
}

// CompiledRule 是规则快照的输入记录，替换快照时统一校验并编译。
type CompiledRule struct {
	// ID 是持久化规则的历史追踪标识。
	ID uint64
	// Type 决定 Pattern 使用关键词、RE2 或组合信号匹配。
	Type RuleType
	// Risk 是规则命中后的风险等级。
	Risk RiskLevel
	// Pattern 是规则正文；组合规则使用 && 分隔全部必需信号。
	Pattern string
}

// RuleSnapshot 是可原子替换的规则集版本。
type RuleSnapshot struct {
	// Version 必须高于当前生效版本。
	Version uint64
	// Rules 会在替换前完整校验并复制为私有运行时结构。
	Rules []CompiledRule
}

// Classification 是一次文本分类的最高风险和精确命中规则集合。
type Classification struct {
	// Risk 是全部命中规则合并后的最高风险。
	Risk RiskLevel
	// RuleMatchIDs 只包含本次实际命中的规则 ID。
	RuleMatchIDs []uint64
	// RulesetVersion 标识本次判定使用的不可变规则快照。
	RulesetVersion uint64
}

// Classifier 对清洗后的纯文本分类，并维护最后一个有效规则快照。
type Classifier interface {
	Classify(processed ProcessedContent) Classification
	ReplaceSnapshot(snapshot RuleSnapshot) error
}
