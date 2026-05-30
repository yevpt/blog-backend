package router

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/internal/handler"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/roles"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Setup 注册所有路由，是整个项目路由的唯一入口
// 新增业务模块时，在此处追加路由组
func Setup(
	r *gin.Engine,
	log *zap.Logger,
	jwtManager *jwt.Manager,
	db *gorm.DB,
	redisClient *redis.Client,
) {
	// 信任所有私有网段作为可信代理。
	// 部署架构：客户端 → 云 Nginx → frp 隧道 → 本地 Docker Go 服务
	// Gin 直接收到的来源是 frpc/Docker 内网 IP（属于私有网段），需要信任它，
	// 才能从 Nginx 写入的 X-Forwarded-For Header 中读到客户端的真实 IP。
	// 涵盖 Docker 默认网段（172.16-31.x.x）、本地回环、内网，无需随环境调整。
	// 安全性保证由 Nginx 端实现：Nginx 用 $remote_addr 覆盖 X-Forwarded-For，
	// 防止客户端伪造该 Header（见 Nginx 配置中的 proxy_set_header 说明）。
	r.SetTrustedProxies([]string{
		"127.0.0.1",
		"::1",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	})

	// ───────────────────────────────────────────
	// 全局中间件
	// ───────────────────────────────────────────
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logger(log))

	// ───────────────────────────────────────────
	// handler 初始化（当前阶段只有测试相关 handler）
	// 后续业务 handler 在此处依次注入依赖
	// ───────────────────────────────────────────
	healthHandler := handler.NewHealthHandler(db, redisClient)
	testHandler := handler.NewTestHandler(jwtManager)

	// ───────────────────────────────────────────
	// ① 公开路由 —— 无需登录，任何人可访问
	// ───────────────────────────────────────────
	r.GET("/health", healthHandler.Check)
	r.GET("/test/public", testHandler.Public)

	// 测试 token 生成（仅非生产环境有效）
	r.POST("/test/token", testHandler.GenToken)

	// ───────────────────────────────────────────
	// ② 登录路由 —— 需要有效 JWT（任意角色）
	// ───────────────────────────────────────────
	authed := r.Group("/", middleware.Auth(jwtManager))
	{
		authed.GET("/test/authed", testHandler.Authed)

		// TODO: 后续在此处添加需要登录的业务路由，例如：
		// authed.POST("/comments", commentHandler.Create)
		// authed.PUT("/user/profile", userHandler.UpdateProfile)
	}

	// ───────────────────────────────────────────
	// ③ VIP 路由 —— 需要 ROLE_VIP 或 ROLE_ADMIN
	// ───────────────────────────────────────────
	vip := r.Group("/", middleware.Auth(jwtManager), middleware.RequireRole(roles.VipRole))
	{
		vip.GET("/test/vip", testHandler.Vip)

		// TODO: 后续在此处添加 VIP 专属路由，例如：
		// vip.GET("/vip/content", contentHandler.GetVipContent)
	}

	// ───────────────────────────────────────────
	// ④ 管理员路由 —— 仅 ROLE_ADMIN 可访问
	// ───────────────────────────────────────────
	admin := r.Group("/admin", middleware.Auth(jwtManager), middleware.RequireRole(roles.AdminRole))
	{
		admin.GET("/test", testHandler.Admin)

		// TODO: 后续在此处添加管理员路由，例如：
		// admin.GET("/users", userHandler.List)
		// admin.DELETE("/articles/:id", articleHandler.Delete)
	}
}
