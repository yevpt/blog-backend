package router

import (
	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	rulemod "github.com/vpt/blog-backend/internal/service/moderationrule"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/storage"
)

func newModerationAdminHandler(
	svc moderationservice.ReviewService,
	userCache userservice.UserCacheService,
	operations moderationservice.OperationsService,
	maxImportFileMB int,
	resolver storage.ObjectURLResolver,
	ruleSvc ...rulemod.Service,
) *moderationhandler.AdminHandler {
	if svc == nil {
		return nil
	}
	handler := moderationhandler.NewAdminHandler(svc, userCache, operations)
	handler.SetRuleImportMaxFileBytes(maxImportFileMB * 1024 * 1024)
	handler.SetObjectURLResolver(resolver)
	if len(ruleSvc) > 0 && ruleSvc[0] != nil {
		handler.SetRuleService(ruleSvc[0])
	}
	return handler
}
