package main

import (
	"github.com/vpt/blog-backend/internal/bootstrap"
	"github.com/vpt/blog-backend/internal/router"
)

// @title Blog Backend API
// @version 1.0
// @description 个人博客后端 API 服务，所有业务接口均使用统一响应结构。
// @BasePath /
func main() {
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

	// 连接缓存：初始化 Redis，用于验证码、限流和对象 URL 缓存。
	redisClient := bootstrap.MustInitRedis(cfg)

	// 初始化认证：创建 JWT 管理器，负责 token 签发和解析。
	jwtManager := bootstrap.InitJWT(cfg)

	// 初始化邮件：创建 SMTP 邮件发送器，负责验证码发送。
	mailer := bootstrap.InitMailer(cfg)

	// 初始化存储：创建 Garage/CDN 对象 URL 解析器，并接入 Redis 缓存。
	objectURLResolver := bootstrap.MustInitStorage(cfg, redisClient)

	// 初始化 HTTP 引擎：设置 Gin 模式并创建空路由引擎。
	r := bootstrap.InitGin(cfg)

	// 注册路由：注入基础设施依赖，并按公开、登录、VIP、admin 分组挂载接口。
	router.Setup(r, zapLogger, jwtManager, db, redisClient, mailer, objectURLResolver, cfg)

	// 启动服务：监听配置端口，启动失败时终止进程。
	bootstrap.MustRunHTTP(r, cfg, zapLogger)
}
