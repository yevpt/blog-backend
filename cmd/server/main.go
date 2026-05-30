package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/router"
	"github.com/vpt/blog-backend/pkg/cache"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/database"
	emailpkg "github.com/vpt/blog-backend/pkg/email"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	zapLogger, err := logger.Init(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		log.Fatalf("日志初始化失败: %v", err)
	}
	defer zapLogger.Sync()

	db, err := database.NewMySQL(&cfg.DB)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	redisClient, err := cache.NewRedis(&cfg.Redis)
	if err != nil {
		log.Fatalf("Redis 连接失败: %v", err)
	}

	jwtManager := jwtpkg.NewManager(cfg.JWT.Secret, cfg.JWT.ExpireHours, cfg.JWT.RefreshExpireHours)

	mailer := emailpkg.NewMailer(&emailpkg.Config{
		Host:     cfg.Email.Host,
		Port:     cfg.Email.Port,
		From:     cfg.Email.From,
		Password: cfg.Email.Password,
	})

	gin.SetMode(cfg.Server.Mode)

	// gin.New() 而非 gin.Default()，由 router.Setup 自定义注入中间件，避免引入默认 Logger/Recovery
	r := gin.New()
	router.Setup(r, zapLogger, jwtManager, db, redisClient, mailer)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	zapLogger.Info(fmt.Sprintf("服务启动，监听 %s (模式: %s)", addr, cfg.Server.Mode))
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
