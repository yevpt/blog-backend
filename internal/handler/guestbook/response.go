package guestbook

import (
	"errors"

	"github.com/gin-gonic/gin"

	guestbookservice "github.com/vpt/blog-backend/internal/service/guestbook"
	"github.com/vpt/blog-backend/pkg/response"
)

func writeGuestbookResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, guestbookservice.ErrGuestbookOwnerNotFound) ||
		errors.Is(err, guestbookservice.ErrGuestbookNotFound) {
		response.NotFound(c)
		return
	}
	if errors.Is(err, guestbookservice.ErrGuestbookNoDeletePermission) {
		response.Forbidden(c)
		return
	}
	if errors.Is(err, guestbookservice.ErrGuestbookInvalid) ||
		errors.Is(err, guestbookservice.ErrGuestbookContentRequired) ||
		errors.Is(err, guestbookservice.ErrGuestbookImageInvalid) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}
