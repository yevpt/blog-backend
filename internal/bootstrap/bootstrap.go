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
	analyticsrepo "github.com/vpt/blog-backend/internal/repository/analytics"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
	analyticsworker "github.com/vpt/blog-backend/internal/worker/analytics"
	moderationworker "github.com/vpt/blog-backend/internal/worker/moderation"
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
		FromName: cfg.Email.FromName,
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
// 站内通知分发始终启动；email.worker_enabled=false 时仅关闭邮件聚合与发送。
func StartNotificationWorker(ctx context.Context, cfg *config.Config, db *gorm.DB, mailer email.MailSender, zapLogger *zap.Logger) {
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
	quota := notificationservice.NewQuotaService(repo, notificationservice.QuotaConfig{
		SiteDailySafeLimit: cfg.Email.SiteDailySafeLimit,
		MaxPerMinute:       cfg.Email.MaxPerMinute,
		MaxPerHour:         cfg.Email.MaxPerHour,
	})
	planner := notificationservice.NewEmailPlanner(repo, quota, directory)
	sender := notificationservice.NewEmailSender(repo, quota, directory, directory, mailer, cfg.Email.Provider, cfg.Email.SiteURL)

	// 组装 worker 运行配置：发送间隔来自配置，分发/聚合用稳健的固定间隔。
	worker := notificationworker.NewWorker(notificationworker.Config{
		Enabled:          true,
		EmailEnabled:     cfg.Email.WorkerEnabled,
		PlannerEnabled:   cfg.Email.PlannerEnabled,
		WorkerID:         notificationWorkerID(),
		BatchSize:        cfg.Email.WorkerBatchSize,
		DispatchInterval: 5 * time.Second,
		PlanInterval:     30 * time.Second,
		SendInterval:     time.Duration(cfg.Email.SendIntervalSeconds) * time.Second,
	}, dispatcher.DispatchOnce, planner.PlanOnce, sender.SendOnce, zapLogger)

	if cfg.Email.WorkerEnabled {
		zapLogger.Info("通知 worker 启动")
	} else {
		zapLogger.Info("站内通知 worker 启动，邮件 worker 未启用")
	}
	go worker.Run(ctx)
}

// StartModerationCleanupWorker 在审核开启时启动有界审计和对象清理。
func StartModerationCleanupWorker(
	ctx context.Context,
	cfg *config.Config,
	db *gorm.DB,
	store storage.ObjectStore,
	zapLogger *zap.Logger,
) {
	if cfg == nil || !cfg.Moderation.Enabled {
		return
	}
	if readable, readableOK := store.(storage.ReadableObjectStore); readableOK {
		recoveryRepo := moderationrepo.NewPublishRecoveryRepository(db)
		recovery := moderationworker.NewPublishRecoveryWorker(
			recoveryRepo, moderationmedia.NewPublisher(readable, moderationrepo.NewRepository(db)), zapLogger,
		)
		zapLogger.Info("碎语图片正式化补偿 worker 启动")
		go recovery.Run(ctx)
	}
	cleanupStore, ok := store.(moderationworker.ObjectStore)
	if !ok {
		zapLogger.Warn("对象存储不支持分页列举，审核清理 worker 未启动")
		return
	}
	worker := moderationworker.NewWorker(
		moderationrepo.NewCleanupRepository(db), cleanupStore, cfg.Moderation, zapLogger, nil,
	)
	zapLogger.Info("审核记录与图片清理 worker 启动")
	go worker.Run(ctx)
}

// StartAnalyticsWorker 启动统计后台：唯一的事件落库消费 goroutine + 日聚合/清理调度器。
//
// 关键约束：ingestor 必须是 router 里 collect handler 投递的同一个实例（经 AnalyticsRuntime 透传），
// 否则消费者会从一个空实例 Run，而生产事件全部堆在另一个实例上被丢弃。
// 本函数全程只调用一次，保证「单 ingestor、单 scheduler」。
// retentionDays / onlineWindow 来自 cfg.Analytics，tz 经 AnalyticsRuntime 透传。
func StartAnalyticsWorker(
	ctx context.Context,
	redisClient *redis.Client,
	zapLogger *zap.Logger,
	ingestor analyticsworker.Ingestor,
	repo analyticsrepo.Repository,
	tz *time.Location,
	retentionDays int,
	onlineWindow time.Duration,
) {
	if tz == nil {
		tz = time.UTC
	}

	// 启动唯一的落库消费循环：消费 collect handler 投递进来的事件。
	go ingestor.Run(ctx)

	// 组装聚合器并启动调度器：每日 00:30 后对昨天执行 RollupDay + 清理，Redis 租约去重。
	rollup := analyticsworker.NewRollup(repo, repo, zapLogger)
	scheduler := analyticsworker.NewScheduler(rollup, redisClient, analyticsworker.SchedulerConfig{
		WorkerID:      notificationWorkerID(),
		TZ:            tz,
		RetentionDays: retentionDays,
		OnlineWindow:  onlineWindow,
		Tick:          time.Minute,
		AfterMinute:   30,
		LeaseTTL:      2 * time.Hour,
	}, zapLogger)
	go scheduler.Run(ctx)

	zapLogger.Info("统计 worker 启动（事件落库 + 日聚合调度）")
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
