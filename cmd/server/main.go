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

	"github.com/vpt/blog-backend/internal/bootstrap"
	"github.com/vpt/blog-backend/internal/router"
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

	// 注册路由：注入基础设施依赖，并按公开、登录、VIP、admin 分组挂载接口。
	// 返回复合运行时（含统计 ingestor 和规则构建 worker），供 main 启动后台任务。
	runtime := router.Setup(r, zapLogger, jwtManager, db, redisClient, mailer, objectURLResolver, cfg)
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	workers := bootstrap.NewTaskGroup(workerCtx, zapLogger)

	// 启动通知后台 worker：事件分发、邮件聚合与发送，依赖 MySQL 租约可恢复。
	bootstrap.StartNotificationWorker(workers, cfg, db, mailer, zapLogger)

	// 启动待审核摘要邮件 worker：先规划批次再发送，复用邮件 worker 总开关。
	bootstrap.StartModerationReviewEmailWorker(workers, cfg, db, mailer, zapLogger)

	// 审核开启后启动有界清理：过期审计、无引用版本、图片记录和孤儿对象。
	bootstrap.StartModerationCleanupWorker(workers, cfg, db, objectURLResolver, zapLogger)

	// 启动统计后台 worker：唯一事件落库消费 + 日聚合/清理调度，与 collect handler 共享同一 ingestor。
	bootstrap.StartAnalyticsWorker(workers, redisClient, zapLogger,
		runtime.Analytics.Ingestor, runtime.Analytics.Repo, runtime.Analytics.TZ,
		cfg.Analytics.RetentionDays, cfg.Analytics.OnlineWindow)

	// 启动规则构建 worker：串行处理候选索引构建和导入校验，与核心审核共享同一分类器。
	if runtime.ModerationRules != nil {
		workers.Go("moderation-rules", runtime.ModerationRules.Run)
		zapLogger.Info("审核规则构建 worker 启动")
	}

	// HTTP 先停止接流量并等待请求完成，再取消后台任务并等待收尾。
	httpErr := bootstrap.RunHTTP(signalCtx, r, cfg, zapLogger)
	cancelWorkers()
	workerShutdownCtx, cancelWorkerShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelWorkerShutdown()
	workerErr := workers.Wait(workerShutdownCtx)
	return errors.Join(httpErr, workerErr)
}
