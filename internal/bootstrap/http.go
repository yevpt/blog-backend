package bootstrap

import (
	"context"
	"errors"
	"fmt"
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
	defaultShutdownTimeout   = 15 * time.Second
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

// RunHTTP 启动 HTTP 服务，并在 context 取消时停止接收请求、等待在途请求完成。
func RunHTTP(ctx context.Context, r *gin.Engine, cfg *config.Config, zapLogger *zap.Logger) error {
	server := newHTTPServer(r, cfg)
	zapLogger.Info("服务启动",
		zap.String("addr", server.Addr),
		zap.String("mode", cfg.Server.Mode),
	)
	return runHTTPServer(ctx, server, zapLogger)
}

func runHTTPServer(ctx context.Context, server *http.Server, zapLogger *zap.Logger) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP 服务运行失败: %w", err)
	case <-ctx.Done():
		zapLogger.Info("收到退出信号，停止接收新请求")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP 服务优雅关闭失败: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP 服务关闭异常: %w", err)
	}
	zapLogger.Info("HTTP 服务已停止")
	return nil
}
