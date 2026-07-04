package router

import (
	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	"github.com/vpt/blog-backend/internal/service/adminlog"
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
	recorder adminlog.Recorder,
	ruleSvc ...rulemod.Service,
) *moderationhandler.AdminHandler {
	if svc == nil {
		return nil
	}
	handler := moderationhandler.NewAdminHandler(svc, userCache, operations)
	handler.SetRuleImportMaxFileBytes(maxImportFileMB * 1024 * 1024)
	handler.SetObjectURLResolver(resolver)
	handler.SetOperationLogRecorder(recorder)
	if len(ruleSvc) > 0 && ruleSvc[0] != nil {
		handler.SetRuleService(ruleSvc[0])
	}
	return handler
}
