package comment

import (
	"errors"

	"github.com/gin-gonic/gin"

	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	"github.com/vpt/blog-backend/pkg/response"
)

func writeCommentResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, commentservice.ErrCommentTargetNotFound) ||
		errors.Is(err, commentservice.ErrCommentNotFound) ||
		errors.Is(err, commentservice.ErrCommentReplyNotFound) {
		response.NotFound(c)
		return
	}
	if errors.Is(err, commentservice.ErrCommentNoDeletePermission) {
		response.Forbidden(c)
		return
	}
	if isCommentBadRequest(err) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}

func isCommentBadRequest(err error) bool {
	return errors.Is(err, commentservice.ErrCommentTargetInvalid) ||
		errors.Is(err, commentservice.ErrCommentContentRequired) ||
		errors.Is(err, commentservice.ErrCommentClosed)
}
