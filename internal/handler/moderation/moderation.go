// Package moderation 提供内容审核的管理员 HTTP 入口。
package moderation

import (
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	rulemod "github.com/vpt/blog-backend/internal/service/moderationrule"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/storage"
)

// AdminHandler 只负责管理端审核参数、身份与响应映射。
type AdminHandler struct {
	svc                    moderationservice.ReviewService
	ops                    moderationservice.OperationsService
	ruleSvc                rulemod.Service
	userCache              userservice.UserCacheService
	ruleImportMaxFileBytes int
	resolver               storage.ObjectURLResolver
}

// SetObjectURLResolver 注入历史图片的管理端访问地址解析器。
func (h *AdminHandler) SetObjectURLResolver(resolver storage.ObjectURLResolver) {
	if h != nil {
		h.resolver = resolver
	}
}

// NewAdminHandler 创建审核管理处理器。
func NewAdminHandler(svc moderationservice.ReviewService, userCache userservice.UserCacheService, operations ...moderationservice.OperationsService) *AdminHandler {
	handler := &AdminHandler{svc: svc, userCache: userCache, ruleImportMaxFileBytes: 50 * 1024 * 1024}
	if len(operations) > 0 {
		handler.ops = operations[0]
	}
	return handler
}

// SetRuleImportMaxFileBytes 注入规则导入的单文件大小上限。
func (h *AdminHandler) SetRuleImportMaxFileBytes(maxBytes int) {
	if h != nil && maxBytes > 0 {
		h.ruleImportMaxFileBytes = maxBytes
	}
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
