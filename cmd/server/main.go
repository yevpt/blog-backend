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

// @title Blog Backend API
// @version 1.0
// @description 个人博客后端 API 服务，所有业务接口均使用统一响应结构。
// @BasePath /
func main() {
	// 加载配置（config.yaml + 环境覆盖 + 环境变量），任何失败均阻断启动
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	// 初始化结构化日志，format/level 由配置决定
	zapLogger, err := logger.Init(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		log.Fatalf("日志初始化失败: %v", err)
	}
	// Sync 在进程退出前刷新缓冲区，避免丢失最后一批日志
	defer zapLogger.Sync()

	// 连接 MySQL，内部已配置连接池参数
	db, err := database.NewMySQL(&cfg.DB)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	// 连接 Redis，内部会 Ping 验证连通性
	redisClient, err := cache.NewRedis(&cfg.Redis)
	if err != nil {
		log.Fatalf("Redis 连接失败: %v", err)
	}

	// 创建 JWT 管理器，持有签名密钥和过期时长配置
	jwtManager := jwtpkg.NewManager(cfg.JWT.Secret, cfg.JWT.ExpireHours, cfg.JWT.RefreshExpireHours)

	// 创建邮件发送器，使用配置中的 SMTP 参数
	mailer := emailpkg.NewMailer(&emailpkg.Config{
		Host:     cfg.Email.Host,
		Port:     cfg.Email.Port,
		From:     cfg.Email.From,
		Password: cfg.Email.Password,
	})

	// 设置 Gin 运行模式（debug 模式打印路由，release 模式关闭调试输出）
	gin.SetMode(cfg.Server.Mode)

	// gin.New() 而非 gin.Default()，由 router.Setup 自定义注入中间件，避免引入默认 Logger/Recovery
	r := gin.New()
	// 注册所有路由、中间件和依赖注入
	router.Setup(r, zapLogger, jwtManager, db, redisClient, mailer)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	zapLogger.Info(fmt.Sprintf("服务启动，监听 %s (模式: %s)", addr, cfg.Server.Mode))
	// 启动 HTTP 服务，阻塞运行直到出错或进程终止
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
