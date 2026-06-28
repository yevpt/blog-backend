package moderation

import (
	"errors"
	"time"
)

var (
	// ErrSubjectNotFound 表示业务内容不存在、已删除或关系不匹配。
	ErrSubjectNotFound = errors.New("moderation subject not found")
	// ErrItemNotFound 表示审核项不存在。
	ErrItemNotFound = errors.New("moderation item not found")
	// ErrOptimisticLock 表示审核项版本已经变化。
	ErrOptimisticLock = errors.New("moderation optimistic lock conflict")
	// ErrPendingRevisionConflict 表示当前待审版本与调用方快照不一致。
	ErrPendingRevisionConflict = errors.New("moderation pending revision conflict")
	// ErrRevisionStateConflict 表示待更新版本已被其他事务处理。
	ErrRevisionStateConflict = errors.New("moderation revision state conflict")
	// ErrIdempotencyDomainConflict 表示同一用户和键同时出现在版本与阻断域。
	ErrIdempotencyDomainConflict = errors.New("moderation idempotency domains conflict")
	// ErrInvalidCommand 表示持久化命令缺少必要数据或包含非法组合。
	ErrInvalidCommand = errors.New("invalid moderation repository command")
)

// SubjectType 是受审核业务内容的封闭类型。
type SubjectType string

const (
	SubjectMoment              SubjectType = "moment"
	SubjectArticleComment      SubjectType = "article_comment"
	SubjectMomentComment       SubjectType = "moment_comment"
	SubjectGuestbook           SubjectType = "guestbook"
	SubjectArticleCommentReply SubjectType = "article_comment_reply"
	SubjectMomentCommentReply  SubjectType = "moment_comment_reply"
	SubjectGuestbookReply      SubjectType = "guestbook_reply"
)

// SubjectRef 唯一定位业务内容及其父关系。
type SubjectRef struct {
	Type SubjectType
	ID   uint64
	// RootID 对一级内容是文章、碎语或留言板 owner；对回复是一级评论 ID。
	RootID uint64
	// ParentID 对回复是 parent_reply_id；nil 表示未提供，指向零表示直接回复一级评论。
	ParentID *uint64
}

// SubjectKey 是不含关系指针、可稳定比较的业务内容标识。
type SubjectKey struct {
	ContentType SubjectType
	ContentID   uint64
}

// Key 返回可安全用于 map key 的业务内容标识。
func (ref SubjectRef) Key() SubjectKey {
	return SubjectKey{ContentType: ref.Type, ContentID: ref.ID}
}

// SubjectSnapshot 是业务正文和归属关系的仓储记录。
type SubjectSnapshot struct {
	Ref      SubjectRef
	AuthorID uint64
	Content  string
}

// LifecycleState 是审核项生命周期。
type LifecycleState string

const (
	LifecycleActive  LifecycleState = "active"
	LifecycleDeleted LifecycleState = "deleted"
)

// PublicState 是审核项公开状态。
type PublicState string

const (
	PublicVisible         PublicState = "visible"
	PublicPlaceholder     PublicState = "placeholder"
	PublicHidden          PublicState = "hidden"
	PublicEmergencyHidden PublicState = "emergency_hidden"
)

// RiskLevel 是版本风险级别。
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// PolicyAction 是版本策略动作。
type PolicyAction string

const (
	ActionAutoApprove PolicyAction = "auto_approve"
	ActionPostReview  PolicyAction = "post_review"
	ActionPreReview   PolicyAction = "pre_review"
	ActionBlock       PolicyAction = "block"
)

// ReviewStatus 是版本审核状态。
type ReviewStatus string

const (
	ReviewPending    ReviewStatus = "pending"
	ReviewApproved   ReviewStatus = "approved"
	ReviewRejected   ReviewStatus = "rejected"
	ReviewSuperseded ReviewStatus = "superseded"
)

// Event 是操作日志事件。
type Event string

const (
	EventSubmit            Event = "submit"
	EventResubmit          Event = "resubmit"
	EventApprove           Event = "approve"
	EventCorrectAndApprove Event = "correct_and_approve"
	EventReject            Event = "reject"
	EventDelete            Event = "delete"
	EventAdminDelete       Event = "admin_delete"
	EventEmergencyHide     Event = "emergency_hide"
	EventRestore           Event = "restore"
	EventTrustChange       Event = "trust_change"
	EventSanctionChange    Event = "sanction_change"
)

// RevisionRef 引用已有版本或本命令新建版本；零值表示 NULL。
type RevisionRef struct {
	ID    uint64
	IsNew bool
}

// NewRevision 返回指向本命令新建版本的引用。
func NewRevision() RevisionRef { return RevisionRef{IsNew: true} }

// ExistingRevision 返回指向已有版本的引用。
func ExistingRevision(id uint64) RevisionRef { return RevisionRef{ID: id} }

// ItemState 是审核项需要持久化的完整目标状态。
type ItemState struct {
	LifecycleState       LifecycleState
	PublicState          PublicState
	Materialized         RevisionRef
	Approved             RevisionRef
	Pending              RevisionRef
	StateBeforeEmergency *PublicState
	EmergencyReason      *string
	EmergencyHiddenAt    *time.Time
	DeletedAt            *time.Time
}

// RevisionDraft 是不可变审核版本的创建数据。
type RevisionDraft struct {
	SubmitterID      uint64
	IdempotencyKey   string
	SubmittedContent string
	PublishedContent string
	RiskLevel        RiskLevel
	PolicyAction     PolicyAction
	ReviewStatus     ReviewStatus
	RulesetVersion   uint64
	RuleMatchIDs     []uint64
	DecisionType     *string
	DecisionReason   *string
	ReviewerID       *uint64
	ReviewedAt       *time.Time
}

// RevisionReview 是已有版本的审核结果更新。
type RevisionReview struct {
	RevisionID uint64
	Status     ReviewStatus
	Decision   string
	Reason     *string
	ReviewerID *uint64
	ReviewedAt time.Time
}

// ActionLog 是事务内追加的审核事实。
type ActionLog struct {
	Revision      RevisionRef
	ActorUserID   *uint64
	SubjectUserID *uint64
	Action        Event
	Reason        *string
	MetadataJSON  *string
	CreatedAt     time.Time
}

// ProfileChange 是事务内对用户审核投影的具名增量。
type ProfileChange struct {
	UserID              uint64
	CleanApprovalDelta  int64
	CorrectedDelta      int64
	RejectedDelta       int64
	HighRiskDelta       int64
	ViolationScoreDelta int64
	TrustLevel          *string
	SanctionState       *string
	UpdatedAt           time.Time
}

// ApplyTransitionCommand 是一次完整审核事务的数据命令。
type ApplyTransitionCommand struct {
	Subject             SubjectRef
	AuthorID            uint64
	ExpectedLockVersion uint64
	ExpectedPendingID   *uint64
	Next                ItemState
	Revision            *RevisionDraft
	SupersedeRevisionID *uint64
	Review              *RevisionReview
	Materialize         RevisionRef
	DeleteSubject       bool
	Log                 *ActionLog
	ProfileChange       *ProfileChange
	// CreateSubject 显式声明首次提交需要在事务内创建业务行。
	CreateSubject bool
}

// AppliedTransition 返回事务提交后的稳定标识。
type AppliedTransition struct {
	Subject         SubjectRef
	ItemID          uint64
	RevisionID      uint64
	RevisionVersion uint64
	LockVersion     uint64
}

// ItemStateRecord 是 service 构建纯状态机输入所需的完整审核项快照。
type ItemStateRecord struct {
	ItemID      uint64
	AuthorID    uint64
	State       ItemState
	LockVersion uint64
}

// MaterializeCommand 是私有适配器的业务表写入命令。
type MaterializeCommand struct {
	Ref      SubjectRef
	AuthorID uint64
	Content  string
	Create   bool
	Visible  bool
	// AssignedID 仅供私有适配器在首次创建后回填自增业务 ID。
	AssignedID *uint64
}

// BlockedAttempt 是高风险阻断的最小审计记录，不包含正文或摘要。
type BlockedAttempt struct {
	UserID         uint64
	SubjectType    SubjectType
	ItemID         *uint64
	IdempotencyKey string
	RulesetVersion uint64
	RuleMatchIDs   []uint64
	CreatedAt      time.Time
}

// ResultKind 区分版本结果与阻断结果。
type ResultKind string

const (
	ResultRevision ResultKind = "revision"
	ResultBlocked  ResultKind = "blocked"
)

// StoredResult 是幂等重放所需的安全结果，不包含审核正文和规则命中。
type StoredResult struct {
	Kind            ResultKind
	RevisionID      uint64
	AttemptID       uint64
	ItemID          uint64
	Subject         SubjectRef
	ReviewStatus    ReviewStatus
	PublicState     PublicState
	CreatedAt       time.Time
	Content         string
	RevisionVersion uint64
	LockVersion     uint64
}

// TrustLevel 是用户审核信任级别。
type TrustLevel string

const (
	TrustNew        TrustLevel = "new"
	TrustNormal     TrustLevel = "normal"
	TrustTrusted    TrustLevel = "trusted"
	TrustRestricted TrustLevel = "restricted"
)

// SanctionState 是用户发布处罚状态。
type SanctionState string

const (
	SanctionActive SanctionState = "active"
	SanctionMuted  SanctionState = "muted"
	SanctionBanned SanctionState = "banned"
)

// PublishingMode 是全站发布控制状态。
type PublishingMode string

const (
	PublishingOpen         PublishingMode = "open"
	PublishingPreReviewAll PublishingMode = "pre_review_all"
	PublishingClosed       PublishingMode = "closed"
)

// PolicyContext 是策略计算需要的数据库快照。
type PolicyContext struct {
	TrustLevel     TrustLevel
	SanctionState  SanctionState
	SanctionUntil  *time.Time
	PublishingMode PublishingMode
	ControlVersion uint64
}

// RuleRecord 是启用规则的不可变值记录。
type RuleRecord struct {
	ID             uint64
	Name           string
	RuleType       string
	Pattern        string
	RiskLevel      RiskLevel
	Priority       int
	RulesetVersion uint64
}

// ViewerRole 决定批量审核视图可返回的字段。
type ViewerRole string

const (
	ViewerPublic ViewerRole = "public"
	ViewerAuthor ViewerRole = "author"
	ViewerAdmin  ViewerRole = "admin"
)

// Viewer 是审核视图读取者。
type Viewer struct {
	Role   ViewerRole
	UserID uint64
}

// View 是业务 service 后续转换 DTO 使用的内部投影。
type View struct {
	PublicState         PublicState
	VisibleContent      string
	HasPendingRevision  bool
	PendingContent      *string
	PendingRiskLevel    *RiskLevel
	PendingReviewStatus *ReviewStatus
	PendingRuleMatchIDs []uint64
	CanInteract         bool
}
