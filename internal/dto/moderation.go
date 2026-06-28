package dto

// ModerationView 描述内容当前公开形态和作者可见的待审状态。
type ModerationView struct {
	// PublicState 是 visible、placeholder、hidden 或 emergency_hidden。
	PublicState string `json:"public_state" example:"visible"`
	// DisplayVersion 表示公开正文来自 pending、last_approved 或 none。
	DisplayVersion string `json:"display_version" example:"pending"`
	// HasPendingRevision 表示当前仍有一个待审版本。
	HasPendingRevision bool `json:"has_pending_revision" example:"true"`
	// PendingRiskLevel 仅向作者和管理员返回。
	PendingRiskLevel *string `json:"pending_risk_level,omitempty" example:"low"`
	// ReviewStatus 仅向作者和管理员返回当前待审状态。
	ReviewStatus *string `json:"review_status,omitempty" example:"pending"`
	// PendingContent 仅向作者和管理员返回，供编辑器显示。
	PendingContent *string `json:"pending_content,omitempty"`
	// CanInteract 表示后端当前是否允许点赞和回复。
	CanInteract bool `json:"can_interact" example:"false"`
}
