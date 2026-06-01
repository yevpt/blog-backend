package guestbook

import (
	"strconv"

	"github.com/gin-gonic/gin"

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
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return 0, false
	}
	return uint(id), true
}

func requiredUser(c *gin.Context) (uint, []string, bool) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return 0, nil, false
	}
	return uint(claims.UserId), claims.Roles, true
}

func optionalUser(c *gin.Context) *uint {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		return nil
	}
	userID := uint(claims.UserId)
	return &userID
}
