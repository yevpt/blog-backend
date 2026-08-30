package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vpt/blog-backend/internal/app"
	"github.com/vpt/blog-backend/internal/bootstrap"
	"go.uber.org/zap"
)

// @title Blog Backend API
// @version 1.0
// @description 个人博客后端 API 服务，所有业务接口均使用统一响应结构。
// @BasePath /
func main() {
	if err := run(); err != nil {
		log.Printf("服务退出异常: %v", err)
		os.Exit(1)
	}
}

func run() error {
	// 信号 context 只触发 HTTP 停止接流量；worker 在请求排空后再取消。
	signalCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	// 加载配置：合并基础配置、环境配置、本地配置和环境变量。
	cfg := bootstrap.MustLoadConfig()

	// 初始化日志：创建 Zap logger，供中间件和启动日志使用。
	zapLogger := bootstrap.MustInitLogger(cfg)
	defer func() {
		// stdout/stderr 在部分系统上会返回无害的 sync 错误，退出前显式忽略。
		_ = zapLogger.Sync()
	}()

	// 连接数据库：初始化 MySQL 与 GORM 连接池。
	db := bootstrap.MustInitMySQL(cfg)
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接池: %w", err)
	}
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			zapLogger.Warn("关闭数据库连接池失败", zap.Error(closeErr))
		}
	}()

	// 连接缓存：初始化 Redis，用于验证码、限流和对象 URL 缓存。
	redisClient := bootstrap.MustInitRedis(cfg)
	defer func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			zapLogger.Warn("关闭 Redis 客户端失败", zap.Error(closeErr))
		}
	}()

	// 初始化认证：创建 JWT 管理器，负责 token 签发和解析。
	jwtManager := bootstrap.InitJWT(cfg)

	// 初始化邮件：创建 SMTP 邮件发送器，负责验证码发送。
	mailer := bootstrap.InitMailer(cfg)

	// 初始化存储：创建 Garage/CDN 对象 URL 解析器，并接入 Redis 缓存。
	objectURLResolver := bootstrap.MustInitStorage(cfg, redisClient)

	// 初始化 HTTP 引擎：设置 Gin 模式并创建空路由引擎。
	r := bootstrap.InitGin(cfg)

	// 构造应用：组合根统一组装仓储、服务、Handler 和后台运行时。
	application, err := app.Build(signalCtx, app.Dependencies{
		Config: cfg, Logger: zapLogger, DB: db, Redis: redisClient,
		JWT: jwtManager, Mailer: mailer, Store: objectURLResolver,
	})
	if err != nil {
		return fmt.Errorf("构造应用: %w", err)
	}
	application.RegisterHTTP(r)

	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	workers := bootstrap.NewTaskGroup(workerCtx, zapLogger)
	application.StartWorkers(workers)

	// HTTP 先停止接流量并等待请求完成，再取消后台任务并等待收尾。
	httpErr := bootstrap.RunHTTP(signalCtx, r, cfg, zapLogger)
	cancelWorkers()
	workerShutdownCtx, cancelWorkerShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelWorkerShutdown()
	workerErr := workers.Wait(workerShutdownCtx)
	return errors.Join(httpErr, workerErr)
}
