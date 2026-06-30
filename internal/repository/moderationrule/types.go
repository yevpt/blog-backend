package moderationrule

import "time"

// RulesetRecord 是当前已发布规则集的启动元数据。
type RulesetRecord struct {
	ID                 uint64
	Status             string
	IndexObjectKey     *string
	IndexFormatVersion uint32
	IndexSHA256        *string
	IndexBytes         uint64
}

// RuleRecord 是构建运行时索引所需的最小规则事实。
type RuleRecord struct {
	ID        uint64
	RuleType  string
	Pattern   string
	RiskLevel string
	Effect    string
	Priority  int32
}

// DedupeHash 是规则去重摘要的定长键。
type DedupeHash [32]byte

// RuleListRecord 是管理端规则列表的一行，包含不可变事实和当前版本的有效状态。
type RuleListRecord struct {
	ID                   uint64
	Name                 *string
	RuleType             string
	Pattern              string
	Category             string
	Effect               string
	RiskLevel            string
	Priority             int32
	SourceID             uint64
	ActivatedRulesetID   uint64
	DeactivatedRulesetID *uint64
	ReplacesRuleID       *uint64
	Active               bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// RuleFilter 定义规则列表的游标、上限和筛选条件。
// ExactPattern 和 PatternPrefix 互斥，同时设置时以 ExactPattern 为准。
type RuleFilter struct {
	AfterID       uint64
	ExactID       uint64
	Limit         int
	ExactPattern  string
	PatternPrefix string
	Category      string
	RuleType      string
	RiskLevel     string
	Effect        string
	SourceID      uint64
	Active        *bool
}

// RulePage 是游标分页结果，NextCursor 为 0 表示没有更多数据。
type RulePage struct {
	Rules      []RuleListRecord
	NextCursor uint64
	HasMore    bool
}

// SourceRecord 是规则来源目录的一行。
type SourceRecord struct {
	ID        uint64
	Name      string
	CreatedAt time.Time
}

// CandidateRecord 是当前候选规则集的构建摘要。
type CandidateRecord struct {
	RulesetID      uint64
	Status         string
	BaseRulesetID  uint64
	RuleCount      uint64
	KeywordCount   uint64
	RegexpCount    uint64
	CompositeCount uint64
	IndexBytes     uint64
	BuildPeakBytes uint64
	FailureCode    *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// StatusRecord 是当前已发布规则集和候选状态的合并摘要。
type StatusRecord struct {
	CurrentRulesetID uint64
	RuleCount        uint64
	KeywordCount     uint64
	RegexpCount      uint64
	CompositeCount   uint64
	IndexBytes       uint64
	BuildPeakBytes   uint64
	BuildDurationMS  uint64
	IndexObjectKey   *string
	IndexSHA256      *string
	UpdatedAt        time.Time
	Candidate        *CandidateRecord
}
