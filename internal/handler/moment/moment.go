package moment

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	"github.com/vpt/blog-backend/internal/middleware"
	momentservice "github.com/vpt/blog-backend/internal/service/moment"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

// MomentHandler 碎语 HTTP 入口，只负责参数绑定、调用 service 和选择响应。
type MomentHandler struct {
	svc momentservice.MomentService
}

// NewMomentHandler 创建碎语 HTTP 处理器。
func NewMomentHandler(svc momentservice.MomentService) *MomentHandler {
	return &MomentHandler{svc: svc}
}

func bindMomentID(c *gin.Context, name string) (uint, bool) {
	return reqbind.PathUint(c, name, "碎语 ID")
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
