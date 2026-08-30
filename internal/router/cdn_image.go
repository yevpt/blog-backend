package router

import (
	"github.com/gin-gonic/gin"
	imagecdnhandler "github.com/vpt/blog-backend/internal/handler/imagecdn"
	"github.com/vpt/blog-backend/internal/middleware"
)

// CDNImageRoute 是应用组合根构造完成的 CDN 图片回源路由。
type CDNImageRoute struct {
	Handler          *imagecdnhandler.Handler
	Bucket           string
	OriginAuthSecret string
}

func registerCDNImageRoutes(engine *gin.Engine, route CDNImageRoute) {
	if route.Handler == nil || route.Bucket == "" || route.OriginAuthSecret == "" {
		return
	}
	engine.GET("/"+route.Bucket+"/*filepath", middleware.OriginAuth(route.OriginAuthSecret), route.Handler.Serve)
}
