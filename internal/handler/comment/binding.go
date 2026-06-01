package comment

import (
	"strconv"

	"github.com/gin-gonic/gin"

	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

func bindCommentID(c *gin.Context, name string) (uint, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return 0, false
	}
	return uint(id), true
}

func requiredCommentClaims(c *gin.Context) (uint, []string, bool) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return 0, nil, false
	}
	return uint(claims.UserId), claims.Roles, true
}
