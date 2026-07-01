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
	SubmitterID          uint64
	IdempotencyKey       string
	SubmittedContent     string
	PublishedContent     string
	RiskLevel            RiskLevel
	PolicyAction         PolicyAction
	ReviewStatus         ReviewStatus
	RulesetVersion       uint64
	RuleMatchIDs         []uint64
	RuleMatchesTruncated bool
	DecisionType         *string
	DecisionReason       *string
	ReviewerID           *uint64
	ReviewedAt           *time.Time
	MomentOptions        *MomentOptions
	Images               []RevisionImageDraft
}

const (
	ImagePending  = "pending"
	ImageApproved = "approved"
)

// ImageFingerprint 是全站图片复用的最终身份，MD5 仅用于缩小候选范围。
type ImageFingerprint struct {
	SHA256 string
	MD5    string
	Size   uint64
}

// PendingImage 是待审图片记录的幂等写入数据。
type PendingImage struct {
	Fingerprint      ImageFingerprint
	PreviewObjectKey string
	LastUsedAt       time.Time
}

// RevisionImageDraft 是新审核版本的有序图片快照。
type RevisionImageDraft struct {
	ImageFingerprint
	Seq       uint
	ObjectKey string
	MediaType string
	IsGIF     bool
}

// RevisionImageRecord 是已持久化的有序图片快照。
type RevisionImageRecord = RevisionImageDraft

// RevisionReview 是已有版本的审核结果更新。
type RevisionReview struct {
	RevisionID       uint64
	Status           ReviewStatus
	Decision         string
	Reason           *string
	ReviewerID       *uint64
	ReviewedAt       time.Time
	PublishedContent *string
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
	ResetCleanApproval  bool
	LastViolationAt     *time.Time
	TrustLevel          *string
	SanctionState       *string
	UpdatedAt           time.Time
}

// NotificationSnapshot 与站内通知 metadata 快照字段对齐。
type NotificationSnapshot struct {
	Type    string `json:"type,omitempty"`
	ID      uint64 `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// ReviewNotificationContext 供审核结果系统通知展示内容归属。
type ReviewNotificationContext struct {
	ContentType                SubjectType
	InteractionRecipientUserID uint64
	CommentID                  *uint64
	RootSnapshot               *NotificationSnapshot
	QuoteSnapshot              *NotificationSnapshot
}

// NotificationIntent 是审核事务内创建站内系统通知所需的最小数据。
type NotificationIntent struct {
	RecipientUserID uint64
	Title           string
	ContentExcerpt  string
	ItemID          uint64
	RevisionID      uint64
	Decision        string
	ContentType     SubjectType
	CommentID       *uint64
	RootSnapshot    *NotificationSnapshot
	QuoteSnapshot   *NotificationSnapshot
}

// InteractionNotificationIntent 是内容首次公开时创建互动通知所需的数据。
type InteractionNotificationIntent struct {
	Type            string
	ActorUserID     uint64
	RecipientUserID uint64
	SourceType      string
	// SourceID 为零时，ApplyTransition 使用本次新物化的业务内容 ID。
	SourceID uint64
	RootType string
	RootID   uint64
	// RootIDFromSubject 表示 RootID 应使用本次新物化的业务内容 ID。
	RootIDFromSubject bool
	ContentExcerpt    string
	CommentID         *uint64
	RootSnapshot      *NotificationSnapshot
	QuoteSnapshot     *NotificationSnapshot
}

// ApplyTransitionCommand 是一次完整审核事务的数据命令。
type ApplyTransitionCommand struct {
	Subject                 SubjectRef
	AuthorID                uint64
	ExpectedLockVersion     uint64
	ExpectedPendingID       *uint64
	Next                    ItemState
	Revision                *RevisionDraft
	SupersedeRevisionID     *uint64
	Review                  *RevisionReview
	Materialize             RevisionRef
	DeleteSubject           bool
	SyncImages              bool
	Log                     *ActionLog
	ProfileChange           *ProfileChange
	Notification            *NotificationIntent
	InteractionNotification *InteractionNotificationIntent
	// MomentOptions 仅用于碎语业务表物化；其他内容类型必须为空。
	MomentOptions *MomentOptions
	// CreateSubject 显式声明首次提交需要在事务内创建业务行。
	CreateSubject bool
}

// MomentOptions 保存碎语本次物化需要保留的业务开关。
type MomentOptions struct {
	Status        uint8
	CommentStatus uint8
}

// ReviewFilter 是管理员审核版本列表的有界筛选条件。
type ReviewFilter struct {
	Page         int
	PageSize     int
	ContentType  *SubjectType
	RiskLevel    *RiskLevel
	ReviewStatus *ReviewStatus
	PublicState  *PublicState
}

// ReviewRecord 汇总审核项状态与一个明确版本，供人工审核 service 使用。
type ReviewRecord struct {
	ItemID           uint64
	Subject          SubjectRef
	AuthorID         uint64
	LockVersion      uint64
	State            ItemState
	RevisionID       uint64
	RevisionVersion  uint64
	SubmittedContent string
	PublishedContent string
	RiskLevel        RiskLevel
	PolicyAction     PolicyAction
	ReviewStatus     ReviewStatus
	MomentOptions    *MomentOptions
	DecisionType     *string
	DecisionReason   *string
	ReviewerID       *uint64
	ReviewedAt       *time.Time
	CreatedAt        time.Time
}

// ReviewPage 是审核版本分页查询结果。
type ReviewPage struct {
	Total int64
	Items []ReviewRecord
}

// ReviewHistoryEvent 是审核项操作日志的只读投影。
type ReviewHistoryEvent struct {
	ID           uint64
	RevisionID   *uint64
	ActorUserID  *uint64
	Action       Event
	Reason       *string
	MetadataJSON *string
	CreatedAt    time.Time
}

// ReviewHistoryPage 聚合一页修订及其图片快照和审核项操作事件。
type ReviewHistoryPage struct {
	Total     int64
	Page      int
	PageSize  int
	Revisions []ReviewRecord
	Images    map[uint64][]RevisionImageRecord
	Events    []ReviewHistoryEvent
}

// AppliedTransition 返回事务提交后的稳定标识。
type AppliedTransition struct {
	Subject         SubjectRef
	ItemID          uint64
	RevisionID      uint64
	RevisionVersion uint64
	LockVersion     uint64
	// Replay 在事务内命中同域幂等结果时返回首次安全结果。
	Replay *StoredResult
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
	// MomentOptions 仅由碎语适配器读取。
	MomentOptions *MomentOptions
	// AssignedID 仅供私有适配器在首次创建后回填自增业务 ID。
	AssignedID *uint64
}

// BlockedAttempt 是高风险阻断的最小审计记录，不包含正文或摘要。
type BlockedAttempt struct {
	UserID               uint64
	SubjectType          SubjectType
	ItemID               *uint64
	IdempotencyKey       string
	RulesetVersion       uint64
	RuleMatchIDs         []uint64
	RuleMatchesTruncated bool
	CreatedAt            time.Time
	// ProfileChange 仅在首次写入该幂等阻断记录时计分。
	ProfileChange *ProfileChange
}

// ResultKind 区分版本结果与阻断结果。
type ResultKind string

const (
	ResultRevision ResultKind = "revision"
	ResultBlocked  ResultKind = "blocked"
)

// StoredResult 是幂等重放所需的安全结果，不包含审核正文和规则命中。
type StoredResult struct {
	Kind         ResultKind
	RevisionID   uint64
	AttemptID    uint64
	ItemID       uint64
	AuthorID     uint64
	Subject      SubjectRef
	ReviewStatus ReviewStatus
	PublicState  PublicState
	CreatedAt    time.Time
	// Content 是本次幂等版本的安全正文，供作者待审回显。
	Content string
	// VisibleContent 是当前实际展示版本，用于中风险编辑稳定重放旧正文。
	VisibleContent  string
	RevisionVersion uint64
	LockVersion     uint64
	RiskLevel       RiskLevel
	PolicyAction    PolicyAction
}

// TrustLevel 是用户审核信任级别。
type TrustLevel string

const (
	TrustNew        TrustLevel = "new"
	TrustNormal     TrustLevel = "normal"
	TrustTrusted    TrustLevel = "trusted"
	TrustRestricted TrustLevel = "restricted"
)

// TrustSource 表示信任等级由系统自动计算还是管理员锁定。
type TrustSource string

const (
	TrustSourceAuto   TrustSource = "auto"
	TrustSourceManual TrustSource = "manual"
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

// RegistrationMode 是全站注册控制状态。
type RegistrationMode string

const (
	RegistrationOpen   RegistrationMode = "open"
	RegistrationClosed RegistrationMode = "closed"
)

// ControlRecord 是全站审核控制单例的仓储投影。
type ControlRecord struct {
	RegistrationMode RegistrationMode
	PublishingMode   PublishingMode
	Reason           *string
	OperatorID       *uint64
	ChangedAt        time.Time
	LockVersion      uint64
}

// UpdateControlCommand 使用乐观锁更新全站审核控制。
type UpdateControlCommand struct {
	RegistrationMode    RegistrationMode
	PublishingMode      PublishingMode
	Reason              *string
	OperatorID          uint64
	ExpectedLockVersion uint64
	ChangedAt           time.Time
}

// UserEmergencyBatchCommand 分批隐藏或恢复一个用户的公开内容。
type UserEmergencyBatchCommand struct {
	UserID  uint64
	ActorID uint64
	Cursor  uint64
	Limit   int
	Hide    bool
	Reason  *string
	Now     time.Time
}

// EmergencyBatchResult 返回本批处理数量和下一游标。
type EmergencyBatchResult struct {
	Processed  int
	NextCursor uint64
	HasMore    bool
}

// PolicyContext 是策略计算需要的数据库快照。
type PolicyContext struct {
	TrustLevel     TrustLevel
	SanctionState  SanctionState
	SanctionUntil  *time.Time
	PublishingMode PublishingMode
	ControlVersion uint64
}

// ModerationProfile 是审核仓储返回的用户治理快照。
type ModerationProfile struct {
	UserID              uint64
	TrustLevel          TrustLevel
	TrustSource         TrustSource
	ManualTrustLocked   bool
	SanctionState       SanctionState
	SanctionUntil       *time.Time
	SanctionReason      *string
	CleanApprovalStreak uint64
	CorrectedCount      uint64
	RejectedCount       uint64
	HighRiskCount       uint64
	ViolationScore      int64
	LastViolationAt     *time.Time
	RestrictedUntil     *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// AutomaticTrustCommand 在未被管理员锁定时更新自动信任等级。
type AutomaticTrustCommand struct {
	UserID          uint64
	TrustLevel      TrustLevel
	RestrictedUntil *time.Time
	UpdatedAt       time.Time
}

// SetTrustCommand 设置管理员校正的信任等级和锁定状态。
type SetTrustCommand struct {
	UserID          uint64
	ActorID         uint64
	TrustLevel      TrustLevel
	ManualLocked    bool
	RestrictedUntil *time.Time
	UpdatedAt       time.Time
}

// SetSanctionCommand 设置用户禁言或封禁；截止时间为空时只能由管理员释放。
type SetSanctionCommand struct {
	UserID  uint64
	ActorID uint64
	State   SanctionState
	Until   *time.Time
	Reason  *string
	Now     time.Time
}

// ReleaseSanctionCommand 记录管理员解除处罚操作。
type ReleaseSanctionCommand struct {
	UserID  uint64
	ActorID uint64
	Now     time.Time
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
	DisplayVersion      DisplayVersion
	VisibleContent      string
	HasPendingRevision  bool
	PendingContent      *string
	PendingRiskLevel    *RiskLevel
	PendingReviewStatus *ReviewStatus
	LastReviewStatus    *ReviewStatus
	PendingRuleMatchIDs []uint64
	VisibleImages       []ImageView
	PendingImages       []ImageView
	CanInteract         bool
}

// ImageView 是审核读取投影中的安全图片地址，未通过时 DisplayObjectKey 只含预览或占位图。
type ImageView struct {
	RevisionImageID  uint64
	Seq              uint
	SourceObjectKey  string
	DisplayObjectKey string
	Approved         bool
	IsGIF            bool
}

// DisplayVersion 表示公开正文选择的审核版本。
type DisplayVersion string

const (
	DisplayPending      DisplayVersion = "pending"
	DisplayLastApproved DisplayVersion = "last_approved"
	DisplayNone         DisplayVersion = "none"
)

// PublishedImageKey 是正式化后单张图片的新对象 key。
type PublishedImageKey struct {
	Seq       uint
	ObjectKey string
}

// AuditImageMove 是被删除公开图片转存私有审计路径的 key 变更。
type AuditImageMove struct {
	OldObjectKey string
	NewObjectKey string
}

// PublishedImageCommand 描述一次审核通过图片正式化的数据库原子更新。
type PublishedImageCommand struct {
	ItemID     uint64
	RevisionID uint64
	MomentID   uint64
	AuthorID   uint64
	ImageKeys  []PublishedImageKey
	AuditMoves []AuditImageMove
}
