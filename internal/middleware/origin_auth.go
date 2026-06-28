package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

const originAuthHeader = "X-Origin-Auth"

// OriginAuth 校验 CDN 回源请求头，密钥不匹配时拒绝访问。
func OriginAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		provided := c.GetHeader(originAuthHeader)
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}
