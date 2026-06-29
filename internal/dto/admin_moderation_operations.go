package dto

import "time"

// AdminModerationControlReq 修改全站审核控制。
type AdminModerationControlReq struct {
	RegistrationMode string `json:"registration_mode" binding:"required,oneof=open closed" example:"open"`
	PublishingMode   string `json:"publishing_mode" binding:"required,oneof=open pre_review_all closed" example:"pre_review_all"`
	Reason           string `json:"reason" example:"临时维护"`
	LockVersion      uint64 `json:"lock_version" binding:"required,min=1" example:"3"`
}

// AdminModerationControlResp 是全站审核控制响应。
type AdminModerationControlResp struct {
	RegistrationMode string    `json:"registration_mode" example:"open"`
	PublishingMode   string    `json:"publishing_mode" example:"open"`
	Reason           *string   `json:"reason,omitempty"`
	OperatorID       *uint64   `json:"operator_id,omitempty"`
	ChangedAt        time.Time `json:"changed_at"`
	LockVersion      uint64    `json:"lock_version" example:"3"`
}

// AdminModerationProfileReq 手工校正用户信任等级。
type AdminModerationProfileReq struct {
	TrustLevel      string     `json:"trust_level" binding:"required,oneof=new normal trusted restricted" example:"trusted"`
	ManualLocked    *bool      `json:"manual_locked" binding:"required"`
	RestrictedUntil *time.Time `json:"restricted_until,omitempty"`
}

// AdminModerationSanctionReq 设置禁言或封禁。
type AdminModerationSanctionReq struct {
	Until  *time.Time `json:"until,omitempty"`
	Reason string     `json:"reason"`
}

// AdminModerationProfileResp 是管理员可见的用户审核画像。
type AdminModerationProfileResp struct {
	UserID              uint64     `json:"user_id"`
	TrustLevel          string     `json:"trust_level"`
	TrustSource         string     `json:"trust_source"`
	ManualTrustLocked   bool       `json:"manual_trust_locked"`
	SanctionState       string     `json:"sanction_state"`
	SanctionUntil       *time.Time `json:"sanction_until,omitempty"`
	SanctionReason      *string    `json:"sanction_reason,omitempty"`
	CleanApprovalStreak uint64     `json:"clean_approval_streak"`
	CorrectedCount      uint64     `json:"corrected_count"`
	RejectedCount       uint64     `json:"rejected_count"`
	HighRiskCount       uint64     `json:"high_risk_count"`
	ViolationScore      int64      `json:"violation_score"`
	LastViolationAt     *time.Time `json:"last_violation_at,omitempty"`
	RestrictedUntil     *time.Time `json:"restricted_until,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// AdminModerationEmergencyReq 是单条紧急隐藏请求。
type AdminModerationEmergencyReq struct {
	Reason string `json:"reason" binding:"required" example:"紧急下架"`
}

// AdminModerationEmergencyBatchReq 是用户内容分批隐藏或恢复请求。
type AdminModerationEmergencyBatchReq struct {
	Cursor uint64 `json:"cursor"`
	Limit  int    `json:"limit" binding:"omitempty,min=1" example:"20"`
	Reason string `json:"reason"`
}

// AdminModerationEmergencyItemResp 是单条紧急操作结果。
type AdminModerationEmergencyItemResp struct {
	ItemID      uint64 `json:"item_id"`
	PublicState string `json:"public_state"`
	LockVersion uint64 `json:"lock_version"`
}

// AdminModerationEmergencyBatchResp 是批量紧急操作进度。
type AdminModerationEmergencyBatchResp struct {
	Processed  int    `json:"processed"`
	NextCursor uint64 `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}
