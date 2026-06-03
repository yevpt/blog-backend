package moment

import (
	"strconv"

	"github.com/gin-gonic/gin"
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
