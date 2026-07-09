package router

import (
	"os"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const corsAllowedOriginsEnv = "CORS_ALLOWED_ORIGINS"

func configureTrustedProxies(r *gin.Engine) {
	// 部署链路：客户端 → 云 Nginx → frp 隧道 → 本地 Docker Go 服务。
	// Nginx 必须覆盖 X-Forwarded-For，防止客户端伪造真实 IP。
	r.SetTrustedProxies([]string{
		"127.0.0.1",
		"::1",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	})
}

func newCORSConfig() cors.Config {
	corsCfg := cors.DefaultConfig()
	allowedOrigins := os.Getenv(corsAllowedOriginsEnv)
	if shouldAllowAllCORSOrigins(allowedOrigins) {
		corsCfg.AllowAllOrigins = true
	} else {
		corsCfg.AllowOrigins = splitCORSOrigins(allowedOrigins)
	}
	corsCfg.AllowHeaders = append(corsCfg.AllowHeaders, "Authorization", "Idempotency-Key")
	return corsCfg
}

func shouldAllowAllCORSOrigins(allowedOrigins string) bool {
	return allowedOrigins == "" || allowedOrigins == "*"
}

func splitCORSOrigins(allowedOrigins string) []string {
	parts := strings.Split(allowedOrigins, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
