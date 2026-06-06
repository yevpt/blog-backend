package comment

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/handler/reqbind"
	"github.com/vpt/blog-backend/internal/middleware"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

func bindCommentID(c *gin.Context, name string) (uint, bool) {
	return reqbind.PathUint(c, name, "评论 ID")
}

func requiredCommentClaims(c *gin.Context) (uint, []string, bool) {
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return 0, nil, false
	}
	return uint(detail.ID), detail.Roles, true
}

func optionalCommentUser(c *gin.Context) *uint {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		return nil
	}
	userID := uint(claims.UserId)
	return &userID
}
