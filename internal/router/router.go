package router

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/internal/handler"
	analyticshandler "github.com/vpt/blog-backend/internal/handler/analytics"
	articlehandler "github.com/vpt/blog-backend/internal/handler/article"
	authhandler "github.com/vpt/blog-backend/internal/handler/auth"
	captchahandler "github.com/vpt/blog-backend/internal/handler/captcha"
	categoryhandler "github.com/vpt/blog-backend/internal/handler/category"
	commenthandler "github.com/vpt/blog-backend/internal/handler/comment"
	dashboardhandler "github.com/vpt/blog-backend/internal/handler/dashboard"
	friendlinkhandler "github.com/vpt/blog-backend/internal/handler/friendlink"
	guestbookhandler "github.com/vpt/blog-backend/internal/handler/guestbook"
	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	momenthandler "github.com/vpt/blog-backend/internal/handler/moment"
	musichandler "github.com/vpt/blog-backend/internal/handler/music"
	notificationhandler "github.com/vpt/blog-backend/internal/handler/notification"
	oauthhandler "github.com/vpt/blog-backend/internal/handler/oauth"
	taghandler "github.com/vpt/blog-backend/internal/handler/tag"
	uploadhandler "github.com/vpt/blog-backend/internal/handler/upload"
	userhandler "github.com/vpt/blog-backend/internal/handler/user"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/jwt"
	"go.uber.org/zap"
)

// Handlers 汇总已经由应用组合根构造完成的 HTTP Handler 与路由依赖。
type Handlers struct {
	Health              *handler.HealthHandler
	Test                *handler.TestHandler
	Auth                *authhandler.AuthHandler
	OAuth               *oauthhandler.OAuthHandler
	Captcha             *captchahandler.CaptchaHandler
	Article             *articlehandler.ArticleHandler
	Comment             *commenthandler.CommentHandler
	Guestbook           *guestbookhandler.GuestbookHandler
	Moment              *momenthandler.MomentHandler
	ModerationAdmin     *moderationhandler.AdminHandler
	Notification        *notificationhandler.NotificationHandler
	NotificationAdmin   *notificationhandler.NotificationAdminHandler
	User                *userhandler.UserHandler
	UserAdmin           *userhandler.UserAdminHandler
	Category            *categoryhandler.CategoryHandler
	Tag                 *taghandler.TagHandler
	Music               *musichandler.MusicHandler
	FriendLink          *friendlinkhandler.FriendLinkHandler
	Upload              *uploadhandler.Handler
	AnalyticsCollect    *analyticshandler.CollectHandler
	AnalyticsAdmin      *analyticshandler.AdminHandler
	AnalyticsPublic     *analyticshandler.PublicHandler
	Dashboard           *dashboardhandler.Handler
	UserCache           middleware.UserDetailLoader
	ModerationRateLimit config.ModerationRateLimitConfig
	CDNImage            CDNImageRoute
}

// Register 只负责全局中间件和路由注册，不创建 repository、service 或 worker。
func Register(
	engine *gin.Engine,
	logger *zap.Logger,
	jwtManager *jwt.Manager,
	redisClient *redis.Client,
	handlers Handlers,
) {
	configureTrustedProxies(engine)
	engine.Use(cors.New(newCORSConfig()))
	engine.Use(middleware.RequestID(), middleware.Recovery(logger), middleware.Logger(logger))
	registerCDNImageRoutes(engine, handlers.CDNImage)
	registerPublicRoutes(engine, handlers, jwtManager, redisClient)
	registerAuthedRoutes(engine, handlers, jwtManager, redisClient)
	registerVIPRoutes(engine, handlers, jwtManager)
	registerAdminRoutes(engine, handlers, jwtManager, redisClient)
}
