package guestbook

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/handler/reqbind"
	"github.com/vpt/blog-backend/internal/middleware"
	guestbookservice "github.com/vpt/blog-backend/internal/service/guestbook"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

// GuestbookHandler 留言板 HTTP 入口，只负责参数绑定、调用 service 和选择响应。
type GuestbookHandler struct {
	svc guestbookservice.GuestbookService
}

// NewGuestbookHandler 创建留言板 HTTP 处理器。
func NewGuestbookHandler(svc guestbookservice.GuestbookService) *GuestbookHandler {
	return &GuestbookHandler{svc: svc}
}

func bindGuestbookID(c *gin.Context, name string) (uint, bool) {
	return reqbind.PathUint(c, name, "留言 ID")
}

func requiredUser(c *gin.Context) (uint, []string, bool) {
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return 0, nil, false
	}
	return uint(detail.ID), detail.Roles, true
}

func optionalUser(c *gin.Context) *uint {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		return nil
	}
	userID := uint(claims.UserId)
	return &userID
}
