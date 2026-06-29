package router

import (
	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
)

func newModerationAdminHandler(svc moderationservice.ReviewService, operations moderationservice.OperationsService) *moderationhandler.AdminHandler {
	if svc == nil {
		return nil
	}
	return moderationhandler.NewAdminHandler(svc, operations)
}
