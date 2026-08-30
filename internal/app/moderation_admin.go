package app

import (
	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	"github.com/vpt/blog-backend/internal/service/adminlog"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	rulemod "github.com/vpt/blog-backend/internal/service/moderationrule"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/storage"
)

func newModerationAdminHandler(
	service moderationservice.ReviewService,
	userCache userservice.UserCacheService,
	operations moderationservice.OperationsService,
	maxImportFileMB int,
	resolver storage.ObjectURLResolver,
	recorder adminlog.Recorder,
	ruleService rulemod.Service,
) *moderationhandler.AdminHandler {
	if service == nil {
		return nil
	}
	handler := moderationhandler.NewAdminHandler(service, userCache, operations)
	handler.SetRuleImportMaxFileBytes(maxImportFileMB * 1024 * 1024)
	handler.SetObjectURLResolver(resolver)
	handler.SetOperationLogRecorder(recorder)
	if ruleService != nil {
		handler.SetRuleService(ruleService)
	}
	return handler
}
