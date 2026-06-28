package comment

import (
	"errors"

	"github.com/gin-gonic/gin"

	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/response"
)

func writeCommentResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, commentservice.ErrCommentTargetNotFound) ||
		errors.Is(err, commentservice.ErrCommentNotFound) ||
		errors.Is(err, commentservice.ErrCommentReplyNotFound) ||
		errors.Is(err, moderationservice.ErrSubjectNotFound) ||
		errors.Is(err, moderationservice.ErrItemNotFound) {
		response.NotFound(c)
		return
	}
	if errors.Is(err, commentservice.ErrCommentNoDeletePermission) {
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
	if isCommentBadRequest(err) {
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
	case errors.Is(err, moderationservice.ErrWriteDisabled):
		response.Fail(c, response.CodeBadRequest, moderationservice.PublicErrorMessage(err))
	default:
		return false
	}
	return true
}

func isCommentBadRequest(err error) bool {
	return errors.Is(err, commentservice.ErrCommentTargetInvalid) ||
		errors.Is(err, commentservice.ErrCommentContentRequired) ||
		errors.Is(err, commentservice.ErrCommentImageInvalid) ||
		errors.Is(err, commentservice.ErrCommentClosed) ||
		errors.Is(err, moderationservice.ErrInvalidRequest) ||
		errors.Is(err, moderationservice.ErrContentTooLong)
}
