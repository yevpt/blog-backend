// Package moderation 提供内容审核的管理员 HTTP 入口。
package moderation

import (
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	rulemod "github.com/vpt/blog-backend/internal/service/moderationrule"
	userservice "github.com/vpt/blog-backend/internal/service/user"
)

// AdminHandler 只负责管理端审核参数、身份与响应映射。
type AdminHandler struct {
	svc       moderationservice.ReviewService
	ops       moderationservice.OperationsService
	ruleSvc   rulemod.Service
	userCache userservice.UserCacheService
}

// NewAdminHandler 创建审核管理处理器。
func NewAdminHandler(svc moderationservice.ReviewService, userCache userservice.UserCacheService, operations ...moderationservice.OperationsService) *AdminHandler {
	handler := &AdminHandler{svc: svc, userCache: userCache}
	if len(operations) > 0 {
		handler.ops = operations[0]
	}
	return handler
}

// SetRuleService 注入规则管理服务，在路由装配时调用。
func (h *AdminHandler) SetRuleService(svc rulemod.Service) {
	if h != nil {
		h.ruleSvc = svc
	}
}

// RulesEnabled 表示规则管理服务已注入。
func (h *AdminHandler) RulesEnabled() bool { return h != nil && h.ruleSvc != nil }

// OperationsEnabled 表示全站治理与紧急处置依赖已完成装配。
func (h *AdminHandler) OperationsEnabled() bool { return h != nil && h.ops != nil }
