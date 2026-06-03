package moment

import (
	"errors"

	"github.com/gin-gonic/gin"
	momentservice "github.com/vpt/blog-backend/internal/service/moment"
	"github.com/vpt/blog-backend/pkg/response"
)

func writeMomentResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, momentservice.ErrMomentNotFound) ||
		errors.Is(err, momentservice.ErrMomentAuthorNotFound) {
		response.NotFound(c)
		return
	}
	if errors.Is(err, momentservice.ErrMomentNoPermission) {
		response.Forbidden(c)
		return
	}
	if isMomentBadRequest(err) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}

func isMomentBadRequest(err error) bool {
	return errors.Is(err, momentservice.ErrMomentInvalid) ||
		errors.Is(err, momentservice.ErrMomentContentRequired) ||
		errors.Is(err, momentservice.ErrMomentTopLimitExceeded)
}
