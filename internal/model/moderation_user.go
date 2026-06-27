package model

import "time"

const (
	ModerationTrustNew          = "new"
	ModerationTrustNormal       = "normal"
	ModerationTrustTrusted      = "trusted"
	ModerationTrustRestricted   = "restricted"
	ModerationTrustSourceAuto   = "auto"
	ModerationTrustSourceManual = "manual"
	ModerationSanctionActive    = "active"
	ModerationSanctionMuted     = "muted"
	ModerationSanctionBanned    = "banned"
)

// UserModerationProfile 是用户信任与处罚状态的查询投影。
type UserModerationProfile struct {
	UserID              uint64     `gorm:"primaryKey"`
	TrustLevel          string     `gorm:"size:16;not null;default:new;index:idx_user_moderation_trust;check:chk_user_moderation_trust,trust_level IN ('new','normal','trusted','restricted')"`
	TrustSource         string     `gorm:"size:16;not null;default:auto;check:chk_user_moderation_trust_source,trust_source IN ('auto','manual')"`
	ManualTrustLocked   bool       `gorm:"type:tinyint(1);not null;default:0"`
	SanctionState       string     `gorm:"size:16;not null;default:active;index:idx_user_moderation_sanction,priority:1;check:chk_user_moderation_sanction,sanction_state IN ('active','muted','banned')"`
	SanctionUntil       *time.Time `gorm:"type:datetime(3);index:idx_user_moderation_sanction,priority:2"`
	SanctionReason      *string    `gorm:"size:1000"`
	CleanApprovalStreak uint64     `gorm:"not null;default:0"`
	CorrectedCount      uint64     `gorm:"not null;default:0"`
	RejectedCount       uint64     `gorm:"not null;default:0"`
	HighRiskCount       uint64     `gorm:"not null;default:0"`
	ViolationScore      int64      `gorm:"not null;default:0"`
	LastViolationAt     *time.Time `gorm:"type:datetime(3)"`
	RestrictedUntil     *time.Time `gorm:"type:datetime(3);index:idx_user_moderation_restricted_until"`
	CreatedAt           time.Time  `gorm:"type:datetime(3);not null"`
	UpdatedAt           time.Time  `gorm:"type:datetime(3);not null"`
	User                User       `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (UserModerationProfile) TableName() string { return "user_moderation_profile" }

// ModerationTrustLevels 返回用户信任等级。
func ModerationTrustLevels() []string {
	return []string{ModerationTrustNew, ModerationTrustNormal, ModerationTrustTrusted, ModerationTrustRestricted}
}

// ModerationTrustSources 返回信任等级来源。
func ModerationTrustSources() []string {
	return []string{ModerationTrustSourceAuto, ModerationTrustSourceManual}
}

// ModerationSanctionStates 返回用户处罚状态。
func ModerationSanctionStates() []string {
	return []string{ModerationSanctionActive, ModerationSanctionMuted, ModerationSanctionBanned}
}
