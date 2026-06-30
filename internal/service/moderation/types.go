package moderation

import "github.com/vpt/blog-backend/internal/service/moderation/ruleindex"

// RiskLevel 表示文本规则判定出的最高风险等级。
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
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

// Classification 是一次文本分类的最高风险和精确命中规则集合。
type Classification struct {
	// Risk 是全部命中规则合并后的最高风险。
	Risk RiskLevel
	// RuleMatchIDs 只包含本次实际命中的规则 ID。
	RuleMatchIDs []uint64
	// RuleMatchesTruncated 表示仍有命中未写入 ID 列表。
	RuleMatchesTruncated bool
	// RulesetVersion 标识本次判定使用的不可变规则快照。
	RulesetVersion uint64
}

// Classifier 对清洗后的纯文本分类，并维护最后一个有效规则快照。
type Classifier interface {
	Classify(processed ProcessedContent) Classification
	ReplaceSnapshot(snapshot *ruleindex.Snapshot) error
}
