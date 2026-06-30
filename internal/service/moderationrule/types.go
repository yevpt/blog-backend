// Package moderationrule 提供审核规则管理服务、候选构建和单实例 worker。
package moderationrule

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/repository/moderationrule"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
)

// ListQuery 是服务层规则列表查询参数。
type ListQuery struct {
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

// CategoryEntry 是受控分类的稳定键和显示名。
type CategoryEntry struct {
	Key  string
	Name string
}

// SourceEntry 是规则来源目录条目。
type SourceEntry struct {
	ID   uint64
	Name string
}

// Metadata 是管理端规则目录元数据。
type Metadata struct {
	Categories []CategoryEntry
	RiskLevels []string
	Effects    []string
	RuleTypes  []string
	Sources    []SourceEntry
}

// Status 是服务层规则集状态摘要。
type Status struct {
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
	Candidate        *CandidateStatus
}

// CandidateStatus 是候选规则集状态摘要。
type CandidateStatus struct {
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

// RuleInput 是新增或替代规则的已校验内容。
type RuleInput struct {
	Name      *string
	RuleType  string
	Pattern   string
	Category  string
	Effect    string
	RiskLevel string
	Priority  int32
	SourceID  uint64
}

// CreateRuleCommand 新增单条规则并触发候选发布。
type CreateRuleCommand struct {
	ExpectedRulesetID uint64
	ActorID           uint64
	Rule              RuleInput
}

// ReplaceRuleCommand 创建替代规则并停用旧规则。
type ReplaceRuleCommand struct {
	RuleID            uint64
	ExpectedRulesetID uint64
	ActorID           uint64
	Rule              RuleInput
}

// BatchStatusCommand 批量启用或停用规则。
type BatchStatusCommand struct {
	ExpectedRulesetID uint64
	ActorID           uint64
	RuleIDs           []uint64
	Active            bool
}

// TestTextCommand 使用当前或候选规则集执行文本试跑。
type TestTextCommand struct {
	Text      string
	RulesetID *uint64
	ActorID   uint64
}

// TestHit 是试跑中单条命中的详情。
type TestHit struct {
	RuleID    uint64
	RuleType  string
	Pattern   string
	Category  string
	RiskLevel string
	Effect    string
	Excerpt   string
}

// TestResult 是文本试跑结果。
type TestResult struct {
	Risk          string
	RulesetID     uint64
	RuleIDs       []uint64
	SuppressedIDs []uint64
	Truncated     bool
	Hits          []TestHit
}

// Job 是异步候选任务的引用。
type Job struct {
	RulesetID     uint64
	BaseRulesetID uint64
	Status        string
}

// CreateImportInput 是创建导入任务的服务层输入。
type CreateImportInput struct {
	FileName         string
	Format           string
	FileSize         uint64
	ObjectKey        string
	SourceName       string
	DefaultCategory  string
	DefaultEffect    string
	DefaultRiskLevel string
	DefaultPriority  int32
	OperatorID       uint64
}

// ImportDefaults 是 CSV/TXT 行值的缺省字段。
type ImportDefaults struct {
	Category  string
	Effect    string
	RiskLevel string
	Priority  int32
}

// Service 是审核规则管理服务接口。
type Service interface {
	ListRules(ctx context.Context, query ListQuery) (moderationrule.RulePage, error)
	Metadata(ctx context.Context) (Metadata, error)
	Status(ctx context.Context) (Status, error)
	CreateRule(ctx context.Context, cmd CreateRuleCommand) (Job, error)
	ReplaceRule(ctx context.Context, cmd ReplaceRuleCommand) (Job, error)
	BatchStatus(ctx context.Context, cmd BatchStatusCommand) (Job, error)
	TestText(ctx context.Context, cmd TestTextCommand) (TestResult, error)
	PublishCandidate(ctx context.Context, rulesetID, expectedBase, actorID uint64) error
	CancelCandidate(ctx context.Context, rulesetID, actorID uint64) error
	CreateImport(ctx context.Context, input CreateImportInput) (moderationrule.ImportRecord, error)
	ListImports(ctx context.Context, afterID uint64, limit int) (moderationrule.ImportPage, error)
	GetImport(ctx context.Context, id uint64) (moderationrule.ImportRecord, error)
	CancelImport(ctx context.Context, id, actorID uint64) error
}

// Worker 是规则集后台构建和导入处理接口。
type Worker interface {
	Run(ctx context.Context)
	ProcessOnce(ctx context.Context) error
}

// SnapshotReplacer 原子替换当前分类器快照，由核心分类器实现。
type SnapshotReplacer interface {
	ReplaceSnapshot(snapshot *ruleindex.Snapshot) error
}

// ClassifierProvider 返回当前分类器以供试跑复用。
type ClassifierProvider interface {
	Classifier() SnapshotReplacer
}
