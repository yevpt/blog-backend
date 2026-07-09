package bootstrap

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
)

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 30 * time.Second
	defaultWriteTimeout      = 60 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	defaultMaxHeaderBytes    = 1 << 20
)

func newHTTPServer(r *gin.Engine, cfg *config.Config) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           r,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		MaxHeaderBytes:    defaultMaxHeaderBytes,
	}
}

// MustRunHTTP 启动 HTTP 服务，失败时终止进程。
func MustRunHTTP(r *gin.Engine, cfg *config.Config, zapLogger *zap.Logger) {
	server := newHTTPServer(r, cfg)
	zapLogger.Info("服务启动",
		zap.String("addr", server.Addr),
		zap.String("mode", cfg.Server.Mode),
	)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("服务启动失败: %v", err)
	}
}
