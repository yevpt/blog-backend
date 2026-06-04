package comment

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/handler/reqbind"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

func bindCommentID(c *gin.Context, name string) (uint, bool) {
	return reqbind.PathUint(c, name, "评论 ID")
}

func requiredCommentClaims(c *gin.Context) (uint, []string, bool) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return 0, nil, false
	}
	return uint(claims.UserId), claims.Roles, true
}
