package moment

import (
	"errors"

	"github.com/gin-gonic/gin"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	momentservice "github.com/vpt/blog-backend/internal/service/moment"
	"github.com/vpt/blog-backend/pkg/response"
)

func writeMomentResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, momentservice.ErrMomentNotFound) ||
		errors.Is(err, momentservice.ErrMomentAuthorNotFound) ||
		errors.Is(err, moderationservice.ErrSubjectNotFound) ||
		errors.Is(err, moderationservice.ErrItemNotFound) {
		response.NotFound(c)
		return
	}
	if errors.Is(err, momentservice.ErrMomentNoPermission) {
		response.Forbidden(c)
		return
	}
	if errors.Is(err, moderationservice.ErrPublishingForbidden) {
		response.ForbiddenWithMessage(c, "当前账号暂时不能发布内容")
		return
	}
	if writeModerationError(c, err) {
		return
	}
	if isMomentBadRequest(err) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}

func writeModerationError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, moderationservice.ErrContentRiskRejected):
		response.UnprocessableEntity(c, response.CodeContentRiskRejected, moderationservice.PublicErrorMessage(err))
	case errors.Is(err, moderationservice.ErrImageReviewUnavailable):
		response.Conflict(c, response.CodeImageReviewUnavailable, moderationservice.PublicErrorMessage(err))
	case errors.Is(err, moderationservice.ErrAlreadyDeleted):
		response.Conflict(c, response.CodeContentAlreadyDeleted, moderationservice.PublicErrorMessage(err))
	case errors.Is(err, moderationservice.ErrInteractionNotAllowed):
		response.Conflict(c, response.CodeContentPendingNoInteraction, moderationservice.PublicErrorMessage(err))
	default:
		return false
	}
	return true
}

func isMomentBadRequest(err error) bool {
	return errors.Is(err, momentservice.ErrMomentInvalid) ||
		errors.Is(err, momentservice.ErrMomentContentRequired) ||
		errors.Is(err, momentservice.ErrMomentTopLimitExceeded) ||
		errors.Is(err, momentservice.ErrMomentImageInvalid) ||
		errors.Is(err, momentservice.ErrMomentImageNotFound) ||
		errors.Is(err, momentservice.ErrMomentImageTooLarge) ||
		errors.Is(err, moderationservice.ErrInvalidRequest) ||
		errors.Is(err, moderationservice.ErrContentTooLong)
}
