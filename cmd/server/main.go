package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/router"
	"github.com/vpt/blog-backend/pkg/cache"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/database"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/logger"
)

func main() {
	// 1. 加载配置（按优先级：config.yaml → config.{env}.yaml → config.local.yaml → 环境变量）
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	// 2. 初始化日志（后续所有组件通过依赖注入使用同一个 logger）
	zapLogger, err := logger.Init(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		log.Fatalf("日志初始化失败: %v", err)
	}
	defer zapLogger.Sync() // 程序退出前刷新缓冲区

	// 3. 初始化 MySQL 数据库连接
	db, err := database.NewMySQL(&cfg.DB)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	// 4. 初始化 Redis 连接
	redisClient, err := cache.NewRedis(&cfg.Redis)
	if err != nil {
		log.Fatalf("Redis 连接失败: %v", err)
	}

	// 5. 初始化 JWT 管理器
	jwtManager := jwtpkg.NewManager(cfg.JWT.Secret, cfg.JWT.ExpireHours, cfg.JWT.RefreshExpireHours)

	// 6. 设置 Gin 运行模式（release 模式关闭调试输出）
	gin.SetMode(cfg.Server.Mode)

	// 7. 初始化 Gin 引擎并注册所有路由
	r := gin.New() // 不使用默认中间件，由 router.Setup 自定义注入
	router.Setup(r, zapLogger, jwtManager, db, redisClient)

	// 8. 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	zapLogger.Info(fmt.Sprintf("服务启动，监听 %s (模式: %s)", addr, cfg.Server.Mode))
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
