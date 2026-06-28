package dto

import "time"

// AdminModerationListReq 是管理端审核版本列表筛选。
type AdminModerationListReq struct {
	Page         int    `form:"page" binding:"omitempty,min=1" example:"1"`
	PageSize     int    `form:"page_size" binding:"omitempty,min=1,max=100" example:"20"`
	ContentType  string `form:"content_type" binding:"omitempty,oneof=moment article_comment moment_comment guestbook article_comment_reply moment_comment_reply guestbook_reply"`
	RiskLevel    string `form:"risk_level" binding:"omitempty,oneof=low medium high"`
	ReviewStatus string `form:"review_status" binding:"omitempty,oneof=pending approved rejected superseded" example:"pending"`
}

// AdminModerationReviewReq 是通过或驳回请求。
type AdminModerationReviewReq struct {
	RevisionID  uint64 `json:"revision_id" binding:"required,min=1" example:"20"`
	LockVersion uint64 `json:"lock_version" binding:"required,min=1" example:"3"`
	Reason      string `json:"reason" example:"内容符合发布要求"`
}

// AdminModerationCorrectReq 是修正后通过请求。
type AdminModerationCorrectReq struct {
	RevisionID  uint64 `json:"revision_id" binding:"required,min=1" example:"20"`
	LockVersion uint64 `json:"lock_version" binding:"required,min=1" example:"3"`
	Content     string `json:"content" binding:"required" example:"管理员修正后的正文"`
	Reason      string `json:"reason" binding:"required" example:"移除不当表述"`
}

// AdminModerationSubjectResp 定位被审核业务内容及其父关系。
type AdminModerationSubjectResp struct {
	Type     string  `json:"type" example:"article_comment"`
	ID       uint64  `json:"id" example:"7"`
	RootID   uint64  `json:"root_id,omitempty" example:"3"`
	ParentID *uint64 `json:"parent_id,omitempty"`
}

// AdminModerationMomentOptionsResp 是待审碎语的业务开关。
type AdminModerationMomentOptionsResp struct {
	Status        uint8 `json:"status" example:"1"`
	CommentStatus uint8 `json:"comment_status" example:"1"`
}

// AdminModerationItemResp 是管理端单个审核版本响应。
type AdminModerationItemResp struct {
	ItemID           uint64                            `json:"item_id" example:"10"`
	Subject          AdminModerationSubjectResp        `json:"subject"`
	AuthorID         uint64                            `json:"author_id" example:"42"`
	LockVersion      uint64                            `json:"lock_version" example:"3"`
	LifecycleState   string                            `json:"lifecycle_state" example:"active"`
	PublicState      string                            `json:"public_state" example:"visible"`
	RevisionID       uint64                            `json:"revision_id" example:"20"`
	RevisionVersion  uint64                            `json:"revision_version" example:"2"`
	SubmittedContent string                            `json:"submitted_content"`
	PublishedContent string                            `json:"published_content"`
	RiskLevel        string                            `json:"risk_level" example:"medium"`
	PolicyAction     string                            `json:"policy_action" example:"pre_review"`
	ReviewStatus     string                            `json:"review_status" example:"pending"`
	MomentOptions    *AdminModerationMomentOptionsResp `json:"moment_options,omitempty"`
	DecisionType     *string                           `json:"decision_type,omitempty"`
	DecisionReason   *string                           `json:"decision_reason,omitempty"`
	ReviewerID       *uint64                           `json:"reviewer_id,omitempty"`
	ReviewedAt       *time.Time                        `json:"reviewed_at,omitempty"`
	CreatedAt        time.Time                         `json:"created_at"`
	CanInteract      bool                              `json:"can_interact"`
}

// AdminModerationPageResp 是管理端审核版本分页响应。
type AdminModerationPageResp struct {
	Total    int64                     `json:"total" example:"12"`
	Page     int                       `json:"page" example:"1"`
	PageSize int                       `json:"page_size" example:"20"`
	List     []AdminModerationItemResp `json:"list"`
}
