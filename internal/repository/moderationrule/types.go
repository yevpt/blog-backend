package moderationrule

import (
	"errors"
	"time"
)

// 仓储层稳定错误，供 service 层映射。
var (
	ErrRulesetConflict   = errors.New("规则集版本冲突")
	ErrCandidateNotFound = errors.New("候选规则集不存在")
)

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
	IndexObjectKey string
	IndexSHA256    string
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

// RuleDraft 是候选规则集中待写入的新规则事实。
type RuleDraft struct {
	Name       *string
	RuleType   string
	Pattern    string
	DedupeHash DedupeHash
	Category   string
	Effect     string
	RiskLevel  string
	Priority   int32
	SourceID   uint64
}

// CreateCandidateCommand 定义创建候选规则集的原子输入。
type CreateCandidateCommand struct {
	BaseRulesetID uint64
	ActorID       uint64
	Additions     []RuleDraft
	RemoveRuleIDs []uint64
}

// BuildResult 是索引构建完成后写回数据库的统计和对象元数据。
type BuildResult struct {
	RuleCount      uint64
	KeywordCount   uint64
	RegexpCount    uint64
	CompositeCount uint64
	IndexBytes     uint64
	BuildPeakBytes uint64
	BuildDurationMS uint64
	IndexObjectKey  string
	IndexSHA256     string
}

// RulesetStatus 常量定义规则集的生命周期状态。
const (
	StatusBuilding   = "building"
	StatusReady      = "ready"
	StatusPublishing = "publishing"
	StatusPublished  = "published"
	StatusFailed     = "failed"
	StatusSuperseded = "superseded"
)

// ImportValidationStatus 常量定义导入校验状态。
const (
	ImportStatusQueued     = "queued"
	ImportStatusValidating = "validating"
	ImportStatusValid      = "valid"
	ImportStatusInvalid    = "invalid"
	ImportStatusCanceled   = "canceled"
)

// ImportRecord 是导入任务的完整摘要。
type ImportRecord struct {
	ID               uint64
	FileName         string
	Format           string
	FileSize         uint64
	ObjectKey        string
	SourceID         uint64
	DefaultCategory  string
	DefaultEffect    string
	DefaultRiskLevel string
	DefaultPriority  int32
	ValidationStatus string
	TotalRows        uint64
	ValidRows        uint64
	DuplicateRows    uint64
	ErrorRows        uint64
	ErrorObjectKey   *string
	RulesetID        *uint64
	OperatorID       uint64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CreateImportCommand 定义创建导入任务的输入。
type CreateImportCommand struct {
	FileName         string
	Format           string
	FileSize         uint64
	ObjectKey        string
	SourceID         uint64
	DefaultCategory  string
	DefaultEffect    string
	DefaultRiskLevel string
	DefaultPriority  int32
	OperatorID       uint64
}

// UpdateImportValidationCommand 定义校验结果写回参数。
type UpdateImportValidationCommand struct {
	ID               uint64
	ValidationStatus string
	TotalRows        uint64
	ValidRows        uint64
	DuplicateRows    uint64
	ErrorRows        uint64
	ErrorObjectKey   *string
	RulesetID        *uint64
}

// ImportPage 是导入历史的游标分页结果。
type ImportPage struct {
	Imports    []ImportRecord
	NextCursor uint64
	HasMore    bool
}
