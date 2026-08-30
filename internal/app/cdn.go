package app

import (
	"context"
	"strings"

	imagecdnhandler "github.com/vpt/blog-backend/internal/handler/imagecdn"
	"github.com/vpt/blog-backend/internal/router"
	imagecdnservice "github.com/vpt/blog-backend/internal/service/imagecdn"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
)

type cdnObjectGetter interface {
	GetImageObject(ctx context.Context, objectName string) ([]byte, error)
}

func newCDNImageRoute(store storage.ObjectStore, cfg *config.Config, logger *zap.Logger) router.CDNImageRoute {
	if cfg == nil || !cfg.Garage.CDN {
		return router.CDNImageRoute{}
	}
	if strings.TrimSpace(cfg.Image.OriginAuthSecret) == "" {
		logger.Warn("CDN 图片回源未启用：image.originAuthSecret 为空")
		return router.CDNImageRoute{}
	}
	getter, ok := store.(cdnObjectGetter)
	if !ok {
		logger.Warn("CDN 图片回源未启用：对象存储不支持 GetImageObject")
		return router.CDNImageRoute{}
	}
	bucket := strings.Trim(cfg.Garage.Bucket, "/")
	if bucket == "" {
		logger.Warn("CDN 图片回源未启用：garage.bucket 为空")
		return router.CDNImageRoute{}
	}
	service := imagecdnservice.NewService(getter, cfg.Image)
	return router.CDNImageRoute{
		Handler: imagecdnhandler.NewHandler(service, cfg), Bucket: bucket, OriginAuthSecret: cfg.Image.OriginAuthSecret,
	}
}
