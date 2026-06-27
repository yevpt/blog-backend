package model

import "time"

const (
	ModerationRegistrationOpen       = "open"
	ModerationRegistrationClosed     = "closed"
	ModerationPublishingOpen         = "open"
	ModerationPublishingPreReviewAll = "pre_review_all"
	ModerationPublishingClosed       = "closed"
)

// ModerationControl 保存全站唯一的注册和发布控制状态。
type ModerationControl struct {
	ID               uint64    `gorm:"primaryKey;check:chk_moderation_control_singleton,id = 1"`
	RegistrationMode string    `gorm:"size:16;not null;default:open;check:chk_moderation_registration_mode,registration_mode IN ('open','closed')"`
	PublishingMode   string    `gorm:"size:24;not null;default:open;check:chk_moderation_publishing_mode,publishing_mode IN ('open','pre_review_all','closed')"`
	Reason           *string   `gorm:"size:1000"`
	OperatorID       *uint64   `gorm:"index:idx_moderation_control_operator"`
	ChangedAt        time.Time `gorm:"type:datetime(3);not null;default:CURRENT_TIMESTAMP(3)"`
	LockVersion      uint64    `gorm:"not null;default:1"`
	CreatedAt        time.Time `gorm:"type:datetime(3);not null"`
	UpdatedAt        time.Time `gorm:"type:datetime(3);not null"`
	Operator         *User     `gorm:"foreignKey:OperatorID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
}

func (ModerationControl) TableName() string { return "moderation_control" }

// ModerationRegistrationModes 返回全站注册模式。
func ModerationRegistrationModes() []string {
	return []string{ModerationRegistrationOpen, ModerationRegistrationClosed}
}

// ModerationPublishingModes 返回全站发布模式。
func ModerationPublishingModes() []string {
	return []string{ModerationPublishingOpen, ModerationPublishingPreReviewAll, ModerationPublishingClosed}
}
