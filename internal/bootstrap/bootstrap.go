package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
	notificationworker "github.com/vpt/blog-backend/internal/worker/notification"
	"github.com/vpt/blog-backend/pkg/cache"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/database"
	"github.com/vpt/blog-backend/pkg/email"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/logger"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MustLoadConfig 加载配置文件和环境变量，失败时终止启动。
func MustLoadConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	return cfg
}

// MustInitLogger 初始化结构化日志，失败时终止启动。
func MustInitLogger(cfg *config.Config) *zap.Logger {
	zapLogger, err := logger.Init(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		log.Fatalf("日志初始化失败: %v", err)
	}
	return zapLogger
}

// MustInitMySQL 连接 MySQL 并配置连接池，失败时终止启动。
func MustInitMySQL(cfg *config.Config) *gorm.DB {
	db, err := database.NewMySQL(&cfg.DB)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	return db
}

// MustInitRedis 连接 Redis 并校验连通性，失败时终止启动。
func MustInitRedis(cfg *config.Config) *redis.Client {
	redisClient, err := cache.NewRedis(&cfg.Redis)
	if err != nil {
		log.Fatalf("Redis 连接失败: %v", err)
	}
	return redisClient
}

// InitJWT 创建 JWT 管理器，用于签发和解析 access/refresh token。
func InitJWT(cfg *config.Config) *jwt.Manager {
	return jwt.NewManager(cfg.JWT.Secret, cfg.JWT.ExpireHours, cfg.JWT.RefreshExpireHours)
}

// InitMailer 创建邮件发送器，用于发送注册和登录验证码。
func InitMailer(cfg *config.Config) email.MailSender {
	return email.NewMailer(&email.Config{
		Host:     cfg.Email.Host,
		Port:     cfg.Email.Port,
		From:     cfg.Email.From,
		Password: cfg.Email.Password,
	})
}

// MustInitStorage 创建对象存储客户端，内部处理 Garage/CDN 签名、上传和 Redis 缓存。
func MustInitStorage(cfg *config.Config, redisClient *redis.Client) storage.ObjectStore {
	objectURLResolver, err := storage.NewCachedGarage(&cfg.Garage, &cfg.CDN, redisClient)
	if err != nil {
		log.Fatalf("对象存储初始化失败: %v", err)
	}
	return objectURLResolver
}

// StartNotificationWorker 组装并在后台启动通知 worker（dispatcher/planner/sender）。
// 未启用时（email.worker_enabled=false）静默跳过；worker 依赖 MySQL 租约，进程退出后可恢复。
func StartNotificationWorker(ctx context.Context, cfg *config.Config, db *gorm.DB, mailer email.MailSender, hub *notificationservice.SSEHub, zapLogger *zap.Logger) {
	if !cfg.Email.WorkerEnabled {
		zapLogger.Info("通知 worker 未启用，跳过启动")
		return
	}

	// 组装数据访问与读侧适配器。
	repo := notificationrepo.NewRepository(db)
	directory := notificationrepo.NewDirectory(db)

	// 组装三条处理链：事件分发、邮件聚合、邮件发送。
	dispatcher := notificationservice.NewDispatcher(
		repo,
		notificationservice.NewRecipientResolver(directory),
		notificationservice.NewPreferenceResolver(repo),
		directory,
	)
	// 共享 HTTP 侧的 SSE hub，分发写入收件箱后实时推送在线用户。
	if hub != nil {
		dispatcher.SetInboxNotifier(hub)
	}
	quota := notificationservice.NewQuotaService(repo, notificationservice.QuotaConfig{
		SiteDailySafeLimit: cfg.Email.SiteDailySafeLimit,
		MaxPerMinute:       cfg.Email.MaxPerMinute,
		MaxPerHour:         cfg.Email.MaxPerHour,
	})
	planner := notificationservice.NewEmailPlanner(repo, quota, directory)
	sender := notificationservice.NewEmailSender(repo, quota, directory, mailer, cfg.Email.Provider)

	// 组装 worker 运行配置：发送间隔来自配置，分发/聚合用稳健的固定间隔。
	worker := notificationworker.NewWorker(notificationworker.Config{
		Enabled:          cfg.Email.WorkerEnabled,
		PlannerEnabled:   cfg.Email.PlannerEnabled,
		WorkerID:         notificationWorkerID(),
		BatchSize:        cfg.Email.WorkerBatchSize,
		DispatchInterval: 5 * time.Second,
		PlanInterval:     30 * time.Second,
		SendInterval:     time.Duration(cfg.Email.SendIntervalSeconds) * time.Second,
	}, dispatcher.DispatchOnce, planner.PlanOnce, sender.SendOnce, zapLogger)

	zapLogger.Info("通知 worker 启动")
	go worker.Run(ctx)
}

// notificationWorkerID 生成 worker 标识，用于任务租约 locked_by，便于多实例区分。
func notificationWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return host + "-" + strconv.Itoa(os.Getpid())
}

// InitGin 设置 Gin 运行模式并创建空引擎，具体中间件由 router.Setup 注册。
func InitGin(cfg *config.Config) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)
	return gin.New()
}

// MustRunHTTP 启动 HTTP 服务，失败时终止进程。
func MustRunHTTP(r *gin.Engine, cfg *config.Config, zapLogger *zap.Logger) {
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	zapLogger.Info(fmt.Sprintf("服务启动，监听 %s (模式: %s)", addr, cfg.Server.Mode))
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
