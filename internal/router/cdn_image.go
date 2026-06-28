package router

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	imagecdnhandler "github.com/vpt/blog-backend/internal/handler/imagecdn"
	"github.com/vpt/blog-backend/internal/middleware"
	imagecdnservice "github.com/vpt/blog-backend/internal/service/imagecdn"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
)

type cdnObjectGetter interface {
	GetObject(ctx context.Context, objectName string) ([]byte, error)
}

// registerCDNImageRoutes 注册腾讯云 CDN 回源图片路由。
func registerCDNImageRoutes(r *gin.Engine, objectStore storage.ObjectStore, cfg *config.Config, log *zap.Logger) {
	if cfg == nil || !cfg.Garage.CDN {
		return
	}
	if strings.TrimSpace(cfg.Image.OriginAuthSecret) == "" {
		if log != nil {
			log.Warn("CDN 图片回源未启用：image.originAuthSecret 为空")
		}
		return
	}

	getter, ok := objectStore.(cdnObjectGetter)
	if !ok {
		if log != nil {
			log.Warn("CDN 图片回源未启用：对象存储不支持 GetObject")
		}
		return
	}

	bucket := strings.Trim(cfg.Garage.Bucket, "/")
	if bucket == "" {
		if log != nil {
			log.Warn("CDN 图片回源未启用：garage.bucket 为空")
		}
		return
	}

	svc := imagecdnservice.NewService(getter, cfg.Image)
	h := imagecdnhandler.NewHandler(svc, cfg)
	r.GET("/"+bucket+"/*filepath", middleware.OriginAuth(cfg.Image.OriginAuthSecret), h.Serve)
}
