// Package moderation 提供内容审核的管理员 HTTP 入口。
package moderation

import moderationservice "github.com/vpt/blog-backend/internal/service/moderation"

// AdminHandler 只负责管理端审核参数、身份与响应映射。
type AdminHandler struct {
	svc moderationservice.ReviewService
	ops moderationservice.OperationsService
}

// NewAdminHandler 创建审核管理处理器。
func NewAdminHandler(svc moderationservice.ReviewService, operations ...moderationservice.OperationsService) *AdminHandler {
	handler := &AdminHandler{svc: svc}
	if len(operations) > 0 {
		handler.ops = operations[0]
	}
	return handler
}

// OperationsEnabled 表示全站治理与紧急处置依赖已完成装配。
func (h *AdminHandler) OperationsEnabled() bool { return h != nil && h.ops != nil }
