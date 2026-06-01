package router

import (
	"os"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/internal/handler"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/internal/service"
	"github.com/vpt/blog-backend/pkg/email"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/roles"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Setup 注册所有路由，是整个项目路由的唯一入口
func Setup(
	r *gin.Engine,
	log *zap.Logger,
	jwtManager *jwt.Manager,
	db *gorm.DB,
	redisClient *redis.Client,
	mailer email.MailSender,
	objectURLResolver service.ObjectURLResolver,
) {
	// 部署链路：客户端 → 云 Nginx → frp 隧道 → 本地 Docker Go 服务
	// Gin 直接接收的来源是 frpc/Docker 内网 IP，需信任私有网段才能读到 Nginx 写入的真实客户端 IP。
	// 安全性由 Nginx 侧保证：Nginx 用 $remote_addr 覆盖 X-Forwarded-For，防止客户端伪造。
	r.SetTrustedProxies([]string{
		"127.0.0.1",
		"::1",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	})

	// CORS 配置：开发环境允许所有来源（*）；生产环境由 Nginx 负责跨域，此处仍保持宽松。
	// 通过环境变量 CORS_ALLOWED_ORIGINS 覆盖，多个来源用逗号分隔。
	allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	corsCfg := cors.DefaultConfig()
	if allowedOrigins == "" || allowedOrigins == "*" {
		corsCfg.AllowAllOrigins = true
	} else {
		parts := strings.Split(allowedOrigins, ",")
		origins := make([]string, 0, len(parts))
		for _, p := range parts {
			if o := strings.TrimSpace(p); o != "" {
				origins = append(origins, o)
			}
		}
		corsCfg.AllowOrigins = origins
	}
	// Authorization header 不在 DefaultConfig 的默认允许列表中，需要显式添加
	corsCfg.AllowHeaders = append(corsCfg.AllowHeaders, "Authorization")
	r.Use(cors.New(corsCfg))

	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logger(log))

	healthHandler := handler.NewHealthHandler(db, redisClient)
	testHandler := handler.NewTestHandler(jwtManager)

	userRepo := repository.NewUserRepository(db)
	authSvc := service.NewAuthService(userRepo, jwtManager, redisClient, mailer)
	authHandler := handler.NewAuthHandler(authSvc)
	articleRepo := repository.NewArticleRepository(db)
	articleSvc := service.NewArticleService(articleRepo, objectURLResolver)
	articleHandler := handler.NewArticleHandler(articleSvc)

	// ① 公开路由
	r.GET("/health", healthHandler.Check)
	r.GET("/test/public", testHandler.Public)
	r.POST("/test/token", testHandler.GenToken)

	// 认证接口独立挂载限流，不放入公开 group 以便精确控制
	r.POST("/auth/send-code", middleware.RateLimitStrict(redisClient), authHandler.SendCode)
	r.POST("/auth/register", middleware.RateLimitStrict(redisClient), authHandler.Register)
	r.POST("/auth/login", middleware.RateLimitNormal(redisClient), authHandler.Login)
	r.POST("/auth/refresh", authHandler.Refresh)
	r.GET("/articles/ids", articleHandler.ListIDs)
	r.GET("/articles", articleHandler.ListPublic)
	r.GET("/articles/:id", middleware.OptionalAuth(jwtManager), articleHandler.GetPublicDetail)
	r.POST("/articles/:id/read", articleHandler.Read)

	// ② 需登录（任意角色）
	authed := r.Group("/", middleware.Auth(jwtManager))
	{
		authed.GET("/test/authed", testHandler.Authed)
		authed.GET("/articles/:id/like", articleHandler.IsLiked)
		authed.POST("/articles/:id/like", articleHandler.ToggleLike)
	}

	// ③ 需 VIP 或更高权限
	vip := r.Group("/", middleware.Auth(jwtManager), middleware.RequireRole(roles.VipRole))
	{
		vip.GET("/test/vip", testHandler.Vip)
	}

	// ④ 仅管理员
	admin := r.Group("/admin", middleware.Auth(jwtManager), middleware.RequireRole(roles.AdminRole))
	{
		admin.GET("/test", testHandler.Admin)
		admin.POST("/articles", articleHandler.Save)
		admin.DELETE("/articles/:id", articleHandler.Delete)
	}
}
