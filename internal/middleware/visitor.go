package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// VisitorIDKey 是 gin.Context 中存储 visitor_id 的 key。
	VisitorIDKey = "visitor_id"
	// VisitorIDCookie 是 Cookie 中 visitor_id 的名称。
	VisitorIDCookie = "visitor_id"
)

// VisitorID 从 Cookie 读取 visitor_id，不存在时生成 UUID v4 并写入 Cookie。
// Cookie 配置：HttpOnly、SameSite=Lax、Max-Age=1 年、Path=/。
func VisitorID() gin.HandlerFunc {
	return func(c *gin.Context) {
		visitorID, err := c.Cookie(VisitorIDCookie)
		if err != nil || visitorID == "" {
			visitorID = uuid.NewString()
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     VisitorIDCookie,
				Value:    visitorID,
				MaxAge:   86400 * 365,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
		c.Set(VisitorIDKey, visitorID)
		c.Next()
	}
}

// GetVisitorID 从 gin.Context 读取 visitor_id。
func GetVisitorID(c *gin.Context) string {
	v, _ := c.Get(VisitorIDKey)
	id, ok := v.(string)
	if !ok {
		return ""
	}
	return id
}