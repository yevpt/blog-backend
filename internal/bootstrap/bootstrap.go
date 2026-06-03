package bootstrap

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
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

// MustInitStorage 创建对象 URL 解析器，内部处理 Garage/CDN 签名和 Redis 缓存。
func MustInitStorage(cfg *config.Config, redisClient *redis.Client) storage.ObjectURLResolver {
	objectURLResolver, err := storage.NewCachedGarage(&cfg.Garage, &cfg.CDN, redisClient)
	if err != nil {
		log.Fatalf("对象存储初始化失败: %v", err)
	}
	return objectURLResolver
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
