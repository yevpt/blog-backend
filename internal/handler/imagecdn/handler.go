package imagecdn

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	appconfig "github.com/vpt/blog-backend/pkg/config"
	imagecdnpkg "github.com/vpt/blog-backend/pkg/imagecdn"
	imagecdnservice "github.com/vpt/blog-backend/internal/service/imagecdn"
)

// Handler 处理 CDN 回源图片请求。
type Handler struct {
	svc    *imagecdnservice.Service
	cfg    *appconfig.Config
	bucket string
}

// NewHandler 创建 CDN 图片 handler。
func NewHandler(svc *imagecdnservice.Service, cfg *appconfig.Config) *Handler {
	bucket := ""
	if cfg != nil {
		bucket = strings.Trim(cfg.Garage.Bucket, "/")
	}
	return &Handler{svc: svc, cfg: cfg, bucket: bucket}
}

// Serve 解析 path 与 query，输出图片响应。
func (h *Handler) Serve(c *gin.Context) {
	if h == nil || h.svc == nil || h.cfg == nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	objectKey, err := imagecdnpkg.ObjectKeyFromCDNPath(h.bucket, c.Request.URL.Path)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	width, quality, transform := imagecdnpkg.ParseTransformParams(c.Request.URL.Query(), h.cfg.Image)
	if err := h.svc.ServeObject(c.Writer, c.Request, objectKey, width, quality, transform); err != nil {
		if errors.Is(err, imagecdnservice.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		if errors.Is(err, imagecdnservice.ErrSourceTooLarge) {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}
}
