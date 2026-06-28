package model

import "time"

const (
	ModerationContentMoment           = "moment"
	ModerationContentArticleComment   = "article_comment"
	ModerationContentMomentComment    = "moment_comment"
	ModerationContentGuestbook        = "guestbook"
	ModerationContentArticleReply     = "article_comment_reply"
	ModerationContentMomentReply      = "moment_comment_reply"
	ModerationContentGuestbookReply   = "guestbook_reply"
	ModerationLifecycleActive         = "active"
	ModerationLifecycleDeleted        = "deleted"
	ModerationPublicVisible           = "visible"
	ModerationPublicPlaceholder       = "placeholder"
	ModerationPublicHidden            = "hidden"
	ModerationPublicEmergencyHidden   = "emergency_hidden"
	ModerationRiskLow                 = "low"
	ModerationRiskMedium              = "medium"
	ModerationRiskHigh                = "high"
	ModerationActionAutoApprove       = "auto_approve"
	ModerationActionPostReview        = "post_review"
	ModerationActionPreReview         = "pre_review"
	ModerationActionBlock             = "block"
	ModerationReviewPending           = "pending"
	ModerationReviewApproved          = "approved"
	ModerationReviewRejected          = "rejected"
	ModerationReviewSuperseded        = "superseded"
	ModerationDecisionApproved        = "approved"
	ModerationDecisionCorrected       = "corrected"
	ModerationDecisionRejected        = "rejected"
	ModerationDecisionLegacyMigration = "legacy_migration"
	ModerationEventSubmit             = "submit"
	ModerationEventResubmit           = "resubmit"
	ModerationEventApprove            = "approve"
	ModerationEventCorrectAndApprove  = "correct_and_approve"
	ModerationEventReject             = "reject"
	ModerationEventDelete             = "delete"
	ModerationEventAdminDelete        = "admin_delete"
	ModerationEventEmergencyHide      = "emergency_hide"
	ModerationEventRestore            = "restore"
	ModerationEventTrustChange        = "trust_change"
	ModerationEventSanctionChange     = "sanction_change"
)

// ModerationItem 保存业务内容当前审核状态和版本指针。
type ModerationItem struct {
	ID                     uint64     `gorm:"primaryKey"`
	ContentType            string     `gorm:"size:40;not null;uniqueIndex:uk_moderation_subject,priority:1;check:chk_moderation_item_content_type,content_type IN ('moment','article_comment','moment_comment','guestbook','article_comment_reply','moment_comment_reply','guestbook_reply')"`
	ContentID              uint64     `gorm:"not null;uniqueIndex:uk_moderation_subject,priority:2"`
	AuthorID               uint64     `gorm:"not null;index:idx_moderation_item_author"`
	LifecycleState         string     `gorm:"size:16;not null;default:active;check:chk_moderation_item_lifecycle,lifecycle_state IN ('active','deleted')"`
	PublicState            string     `gorm:"size:24;not null;default:placeholder;check:chk_moderation_item_public_state,public_state IN ('visible','placeholder','hidden','emergency_hidden')"`
	MaterializedRevisionID *uint64    `gorm:"index:idx_moderation_item_materialized_revision"`
	ApprovedRevisionID     *uint64    `gorm:"index:idx_moderation_item_approved_revision"`
	PendingRevisionID      *uint64    `gorm:"index:idx_moderation_item_pending_revision"`
	StateBeforeEmergency   *string    `gorm:"size:24;check:chk_moderation_item_previous_state,state_before_emergency IS NULL OR state_before_emergency IN ('visible','placeholder','hidden')"`
	EmergencyHiddenReason  *string    `gorm:"size:1000"`
	EmergencyHiddenAt      *time.Time `gorm:"type:datetime(3)"`
	DeletedAt              *time.Time `gorm:"type:datetime(3);index:idx_moderation_item_deleted_at"`
	LockVersion            uint64     `gorm:"not null;default:1"`
	CreatedAt              time.Time  `gorm:"type:datetime(3);not null"`
	UpdatedAt              time.Time  `gorm:"type:datetime(3);not null"`
}

func (ModerationItem) TableName() string { return "moderation_item" }

// ModerationRevision 保存不可变提交原文和可修正的发布正文。
type ModerationRevision struct {
	ID                  uint64          `gorm:"primaryKey"`
	ItemID              uint64          `gorm:"not null;uniqueIndex:uk_moderation_revision_version,priority:1;index:idx_moderation_revision_item_status,priority:1"`
	Version             uint64          `gorm:"not null;uniqueIndex:uk_moderation_revision_version,priority:2"`
	SubmitterID         uint64          `gorm:"not null;uniqueIndex:uk_moderation_revision_idempotency,priority:1;index:idx_moderation_revision_submitter"`
	IdempotencyKey      string          `gorm:"size:128;not null;uniqueIndex:uk_moderation_revision_idempotency,priority:2"`
	SubmittedContent    string          `gorm:"type:longtext;not null"`
	PublishedContent    string          `gorm:"type:longtext;not null"`
	RiskLevel           string          `gorm:"size:16;not null;check:chk_moderation_revision_risk,risk_level IN ('low','medium','high')"`
	PolicyAction        string          `gorm:"size:24;not null;check:chk_moderation_revision_policy,policy_action IN ('auto_approve','post_review','pre_review','block')"`
	ReviewStatus        string          `gorm:"size:16;not null;index:idx_moderation_revision_item_status,priority:2;index:idx_moderation_revision_queue,priority:1;check:chk_moderation_revision_status,review_status IN ('pending','approved','rejected','superseded')"`
	MomentStatus        *uint8          `gorm:"type:tinyint;comment:碎语提交时公开开关"`
	MomentCommentStatus *uint8          `gorm:"type:tinyint;comment:碎语提交时评论开关"`
	RulesetVersion      uint64          `gorm:"not null"`
	RuleMatchIDs        string          `gorm:"type:json;not null"`
	DecisionType        *string         `gorm:"size:24;check:chk_moderation_revision_decision,decision_type IS NULL OR decision_type IN ('approved','corrected','rejected','legacy_migration')"`
	DecisionReason      *string         `gorm:"size:1000"`
	ReviewerID          *uint64         `gorm:"index:idx_moderation_revision_reviewer"`
	ReviewedAt          *time.Time      `gorm:"type:datetime(3)"`
	CreatedAt           time.Time       `gorm:"type:datetime(3);not null;index:idx_moderation_revision_queue,priority:2"`
	UpdatedAt           time.Time       `gorm:"type:datetime(3);not null"`
	Item                *ModerationItem `gorm:"foreignKey:ItemID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ModerationRevision) TableName() string { return "moderation_revision" }

// ModerationAttempt 仅保存高风险阻断所需的最小审计信息。
type ModerationAttempt struct {
	ID             uint64          `gorm:"primaryKey"`
	UserID         uint64          `gorm:"not null;uniqueIndex:uk_moderation_attempt_idempotency,priority:1;index:idx_moderation_attempt_user_created,priority:1"`
	ContentType    string          `gorm:"size:40;not null;check:chk_moderation_attempt_content_type,content_type IN ('moment','article_comment','moment_comment','guestbook','article_comment_reply','moment_comment_reply','guestbook_reply')"`
	ItemID         *uint64         `gorm:"index:idx_moderation_attempt_item"`
	IdempotencyKey string          `gorm:"size:128;not null;uniqueIndex:uk_moderation_attempt_idempotency,priority:2"`
	RulesetVersion uint64          `gorm:"not null"`
	RuleMatchIDs   string          `gorm:"type:json;not null"`
	CreatedAt      time.Time       `gorm:"type:datetime(3);not null;index:idx_moderation_attempt_user_created,priority:2"`
	Item           *ModerationItem `gorm:"foreignKey:ItemID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ModerationAttempt) TableName() string { return "moderation_attempt" }

// ModerationActionLog 追加记录审核状态转换和用户治理动作。
type ModerationActionLog struct {
	ID            uint64              `gorm:"primaryKey"`
	ItemID        *uint64             `gorm:"index:idx_moderation_action_item_created,priority:1"`
	RevisionID    *uint64             `gorm:"index:idx_moderation_action_revision"`
	ActorUserID   *uint64             `gorm:"index:idx_moderation_action_actor"`
	SubjectUserID *uint64             `gorm:"index:idx_moderation_action_subject_user"`
	Action        string              `gorm:"size:32;not null;check:chk_moderation_action,action IN ('submit','resubmit','approve','correct_and_approve','reject','delete','admin_delete','emergency_hide','restore','trust_change','sanction_change')"`
	Reason        *string             `gorm:"size:1000"`
	MetadataJSON  *string             `gorm:"type:json"`
	CreatedAt     time.Time           `gorm:"type:datetime(3);not null;index:idx_moderation_action_item_created,priority:2"`
	Item          *ModerationItem     `gorm:"foreignKey:ItemID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Revision      *ModerationRevision `gorm:"foreignKey:RevisionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ModerationActionLog) TableName() string { return "moderation_action_log" }

// ModerationVisibleImage 保存当前物化到非碎语业务内容的图片顺序。
type ModerationVisibleImage struct {
	ID         uint64             `gorm:"primaryKey"`
	ItemID     uint64             `gorm:"not null;uniqueIndex:uk_moderation_visible_image_seq,priority:1"`
	RevisionID uint64             `gorm:"not null;index:idx_moderation_visible_image_revision"`
	Seq        uint               `gorm:"not null;uniqueIndex:uk_moderation_visible_image_seq,priority:2"`
	ObjectKey  string             `gorm:"size:500;not null"`
	CreatedAt  time.Time          `gorm:"type:datetime(3);not null"`
	UpdatedAt  time.Time          `gorm:"type:datetime(3);not null"`
	Item       ModerationItem     `gorm:"foreignKey:ItemID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Revision   ModerationRevision `gorm:"foreignKey:RevisionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ModerationVisibleImage) TableName() string { return "moderation_visible_image" }

// ModerationLifecycleStates 返回可持久化的生命周期状态。
func ModerationLifecycleStates() []string {
	return []string{ModerationLifecycleActive, ModerationLifecycleDeleted}
}

// ModerationPublicStates 返回可持久化的公开状态。
func ModerationPublicStates() []string {
	return []string{ModerationPublicVisible, ModerationPublicPlaceholder, ModerationPublicHidden, ModerationPublicEmergencyHidden}
}

// ModerationRiskLevels 返回可持久化的风险等级。
func ModerationRiskLevels() []string {
	return []string{ModerationRiskLow, ModerationRiskMedium, ModerationRiskHigh}
}

// ModerationPolicyActions 返回可持久化的策略动作。
func ModerationPolicyActions() []string {
	return []string{ModerationActionAutoApprove, ModerationActionPostReview, ModerationActionPreReview, ModerationActionBlock}
}

// ModerationReviewStatuses 返回可持久化的审核状态。
func ModerationReviewStatuses() []string {
	return []string{ModerationReviewPending, ModerationReviewApproved, ModerationReviewRejected, ModerationReviewSuperseded}
}

// ModerationDecisionTypes 返回人工决策类型。
func ModerationDecisionTypes() []string {
	return []string{ModerationDecisionApproved, ModerationDecisionCorrected, ModerationDecisionRejected, ModerationDecisionLegacyMigration}
}
