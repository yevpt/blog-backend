// Package moderation 提供内容审核的管理员 HTTP 入口。
package moderation

import moderationservice "github.com/vpt/blog-backend/internal/service/moderation"

// AdminHandler 只负责管理端审核参数、身份与响应映射。
type AdminHandler struct {
	svc moderationservice.ReviewService
}

// NewAdminHandler 创建审核管理处理器。
func NewAdminHandler(svc moderationservice.ReviewService) *AdminHandler {
	return &AdminHandler{svc: svc}
}
