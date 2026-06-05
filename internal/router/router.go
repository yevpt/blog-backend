package router

import (
	"os"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/internal/handler"
	articlehandler "github.com/vpt/blog-backend/internal/handler/article"
	authhandler "github.com/vpt/blog-backend/internal/handler/auth"
	captchahandler "github.com/vpt/blog-backend/internal/handler/captcha"
	commenthandler "github.com/vpt/blog-backend/internal/handler/comment"
	guestbookhandler "github.com/vpt/blog-backend/internal/handler/guestbook"
	momenthandler "github.com/vpt/blog-backend/internal/handler/moment"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/internal/repository"
	articlerepo "github.com/vpt/blog-backend/internal/repository/article"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
	"github.com/vpt/blog-backend/internal/service"
	articleservice "github.com/vpt/blog-backend/internal/service/article"
	authservice "github.com/vpt/blog-backend/internal/service/auth"
	captchaservice "github.com/vpt/blog-backend/internal/service/captcha"
	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	guestbookservice "github.com/vpt/blog-backend/internal/service/guestbook"
	momentservice "github.com/vpt/blog-backend/internal/service/moment"
	"github.com/vpt/blog-backend/pkg/email"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const corsAllowedOriginsEnv = "CORS_ALLOWED_ORIGINS"

type routeHandlers struct {
	health     *handler.HealthHandler
	test       *handler.TestHandler
	auth       *authhandler.AuthHandler
	captcha    *captchahandler.CaptchaHandler
	article    *articlehandler.ArticleHandler
	comment    *commenthandler.CommentHandler
	guestbook  *guestbookhandler.GuestbookHandler
	moment     *momenthandler.MomentHandler
	user       *handler.UserHandler
	category   *handler.CategoryHandler
	tag        *handler.TagHandler
	userCache  service.UserCacheService
}

// Setup 注册所有路由，是整个项目路由的唯一入口
func Setup(
	r *gin.Engine,
	log *zap.Logger,
	jwtManager *jwt.Manager,
	db *gorm.DB,
	redisClient *redis.Client,
	mailer email.MailSender,
	objectURLResolver storage.ObjectURLResolver,
) {
	// 配置信任代理，确保反向代理链路下能拿到真实客户端 IP。
	configureTrustedProxies(r)

	// 注册跨域中间件，支持开发环境和生产代理环境的来源策略。
	r.Use(cors.New(newCORSConfig()))

	// 注册全局基础中间件，统一处理恢复和请求日志。
	r.Use(middleware.Recovery(log), middleware.Logger(log))

	// 组装路由所需的 handler，保持 Setup 只关心注册流程。
	handlers := newRouteHandlers(db, redisClient, jwtManager, mailer, objectURLResolver)

	// 按权限层级注册路由，公开路由在前，受保护路由在后。
	registerPublicRoutes(r, handlers, jwtManager, redisClient)
	registerAuthedRoutes(r, handlers, jwtManager)
	registerVIPRoutes(r, handlers, jwtManager)
	registerAdminRoutes(r, handlers, jwtManager)
}

func configureTrustedProxies(r *gin.Engine) {
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
}

func newCORSConfig() cors.Config {
	// CORS 配置：开发环境允许所有来源（*）；生产环境由 Nginx 负责跨域，此处仍保持宽松。
	// 通过环境变量 CORS_ALLOWED_ORIGINS 覆盖，多个来源用逗号分隔。
	corsCfg := cors.DefaultConfig()
	allowedOrigins := os.Getenv(corsAllowedOriginsEnv)

	// 解析允许来源，空值和星号都表示放开来源。
	if shouldAllowAllCORSOrigins(allowedOrigins) {
		corsCfg.AllowAllOrigins = true
	} else {
		corsCfg.AllowOrigins = splitCORSOrigins(allowedOrigins)
	}

	// Authorization header 不在 DefaultConfig 的默认允许列表中，需要显式添加
	corsCfg.AllowHeaders = append(corsCfg.AllowHeaders, "Authorization")

	return corsCfg
}

func shouldAllowAllCORSOrigins(allowedOrigins string) bool {
	// 空值和星号沿用原有宽松策略。
	if allowedOrigins == "" || allowedOrigins == "*" {
		return true
	}

	return false
}

func splitCORSOrigins(allowedOrigins string) []string {
	// 拆分多个来源，并丢弃误填的空白项。
	parts := strings.Split(allowedOrigins, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}

	return origins
}

func newRouteHandlers(
	db *gorm.DB,
	redisClient *redis.Client,
	jwtManager *jwt.Manager,
	mailer email.MailSender,
	objectURLResolver storage.ObjectURLResolver,
) routeHandlers {
	// 组装图形验证码链路，注册发送邮箱验证码前会消费它签发的一次性票据。
	captchaSvc, err := captchaservice.NewService(redisClient)
	if err != nil {
		panic(err)
	}

	// 组装认证链路，保持依赖从 repository 到 service 再到 handler 的方向。
	userRepo := repository.NewUserRepository(db)
	userCacheSvc := service.NewUserCacheService(userRepo, objectURLResolver, redisClient)
	authSvc := authservice.NewAuthService(userRepo, jwtManager, redisClient, mailer, captchaSvc, userCacheSvc)
	userSvc := service.NewUserService(userCacheSvc)

	// 组装文章链路，前端对象地址由 service 层统一解析。
	articleRepo := articlerepo.NewArticleRepository(db)
	articleSvc := articleservice.NewArticleService(articleRepo, objectURLResolver)

	categoryRepo := repository.NewCategoryRepository(db)
	categorySvc := service.NewCategoryService(categoryRepo)

	tagRepo := repository.NewTagRepository(db)
	tagSvc := service.NewTagService(tagRepo, articleSvc)

	commentRepo := commentrepo.NewCommentRepository(db)
	commentSvc := commentservice.NewCommentService(commentRepo, objectURLResolver)

	guestbookRepo := guestbookrepo.NewGuestbookRepository(db)
	guestbookSvc := guestbookservice.NewGuestbookService(guestbookRepo)

	momentRepo := momentrepo.NewMomentRepository(db)
	momentSvc := momentservice.NewMomentService(momentRepo, objectURLResolver)

	return routeHandlers{
		health:    handler.NewHealthHandler(db, redisClient),
		test:      handler.NewTestHandler(jwtManager),
		auth:      authhandler.NewAuthHandler(authSvc),
		captcha:   captchahandler.NewCaptchaHandler(captchaSvc),
		article:   articlehandler.NewArticleHandler(articleSvc),
		comment:   commenthandler.NewCommentHandler(commentSvc),
		guestbook: guestbookhandler.NewGuestbookHandler(guestbookSvc),
		moment:    momenthandler.NewMomentHandler(momentSvc),
		user:      handler.NewUserHandler(userSvc),
		category:  handler.NewCategoryHandler(categorySvc),
		tag:       handler.NewTagHandler(tagSvc),
		userCache: userCacheSvc,
	}
}

func registerPublicRoutes(
	r *gin.Engine,
	handlers routeHandlers,
	jwtManager *jwt.Manager,
	redisClient *redis.Client,
) {
	// 公开路由直接挂载，保留 URL 与 handler 的显式对应关系。
	r.GET("/health", handlers.health.Check)
	r.GET("/test/public", handlers.test.Public)
	r.POST("/test/token", handlers.test.GenToken)

	// 认证接口独立挂载限流，不放入公开 group 以便精确控制
	r.POST("/captcha/register/challenge", handlers.captcha.GenerateRegistrationChallenge)
	r.POST("/captcha/register/verify", handlers.captcha.VerifyRegistrationChallenge)
	r.POST("/auth/send-code", middleware.RateLimitStrict(redisClient), handlers.auth.SendCode)
	r.POST("/auth/register", middleware.RateLimitStrict(redisClient), handlers.auth.Register)
	r.POST("/auth/login", middleware.RateLimitNormal(redisClient), handlers.auth.Login)
	r.POST("/auth/refresh", handlers.auth.Refresh)
	r.GET("/categories", handlers.category.ListTabs)
	r.GET("/tags", handlers.tag.List)
	r.GET("/tags/:id", handlers.tag.Get)
	r.GET("/tags/:id/articles", handlers.tag.ListArticles)
	r.GET("/articles/ids", handlers.article.ListIDs)
	r.GET("/articles", handlers.article.ListPublic)
	r.GET("/articles/:id", middleware.OptionalAuth(jwtManager), handlers.article.GetPublicDetail)
	r.POST("/articles/:id/read", handlers.article.Read)
	r.GET("/comments", handlers.comment.List)
	r.GET("/guestbook", middleware.OptionalAuth(jwtManager), handlers.guestbook.List)
	r.GET("/moments", middleware.OptionalAuth(jwtManager), handlers.moment.List)
	r.GET("/moments/:id", middleware.OptionalAuth(jwtManager), handlers.moment.GetDetail)
	r.POST("/moments/:id/read", handlers.moment.Read)
}

func registerAuthedRoutes(r *gin.Engine, handlers routeHandlers, jwtManager *jwt.Manager) {
	// 登录路由要求任意已认证用户。
	authed := r.Group("/", middleware.Auth(jwtManager, handlers.userCache))
	authed.GET("/test/authed", handlers.test.Authed)
	authed.GET("/users/me", handlers.user.GetDetail)
	authed.GET("/articles/:id/like", handlers.article.IsLiked)
	authed.POST("/articles/:id/like", handlers.article.ToggleLike)
	authed.POST("/comments", handlers.comment.Create)
	authed.POST("/comments/:id/replies", handlers.comment.Reply)
	authed.DELETE("/comments/:id", handlers.comment.Delete)
	authed.DELETE("/comment-replies/:id", handlers.comment.DeleteReply)
	authed.POST("/guestbook", handlers.guestbook.Create)
	authed.POST("/guestbook/:id/like", handlers.guestbook.ToggleLike)
	authed.DELETE("/guestbook/:id", handlers.guestbook.Delete)
	authed.POST("/moments", handlers.moment.Save)
	authed.DELETE("/moments/:id", handlers.moment.Delete)
	authed.POST("/moments/:id/top", handlers.moment.SetTop)
	authed.DELETE("/moments/:id/top", handlers.moment.RemoveTop)
	authed.GET("/moments/:id/like", handlers.moment.IsLiked)
	authed.POST("/moments/:id/like", handlers.moment.ToggleLike)
}

func registerVIPRoutes(r *gin.Engine, handlers routeHandlers, jwtManager *jwt.Manager) {
	// VIP 路由要求 VIP 或更高权限。
	vip := r.Group("/", middleware.Auth(jwtManager, handlers.userCache), middleware.RequireRole(roles.VipRole))
	vip.GET("/test/vip", handlers.test.Vip)
}

func registerAdminRoutes(r *gin.Engine, handlers routeHandlers, jwtManager *jwt.Manager) {
	// 管理员路由统一挂在 /admin 前缀下。
	admin := r.Group("/admin", middleware.Auth(jwtManager, handlers.userCache), middleware.RequireRole(roles.AdminRole))
	admin.GET("/test", handlers.test.Admin)
	admin.POST("/articles", handlers.article.Save)
	admin.DELETE("/articles/:id", handlers.article.Delete)
	admin.POST("/categories", handlers.category.Create)
	admin.PUT("/categories/:id", handlers.category.Update)
	admin.DELETE("/categories/:id", handlers.category.Delete)
	admin.POST("/categories/:id/articles", handlers.category.AddArticles)
	admin.DELETE("/categories/:id/articles", handlers.category.RemoveArticles)
	admin.POST("/tags", handlers.tag.Create)
	admin.PUT("/tags/:id", handlers.tag.Update)
	admin.DELETE("/tags/:id", handlers.tag.Delete)
	admin.POST("/tags/:id/articles", handlers.tag.AddArticles)
	admin.DELETE("/tags/:id/articles", handlers.tag.RemoveArticles)
}
