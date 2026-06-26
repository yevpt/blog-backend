package router

import (
	"os"
	"strings"

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
	momenthandler "github.com/vpt/blog-backend/internal/handler/moment"
	musichandler "github.com/vpt/blog-backend/internal/handler/music"
	notificationhandler "github.com/vpt/blog-backend/internal/handler/notification"
	oauthhandler "github.com/vpt/blog-backend/internal/handler/oauth"
	taghandler "github.com/vpt/blog-backend/internal/handler/tag"
	uploadhandler "github.com/vpt/blog-backend/internal/handler/upload"
	userhandler "github.com/vpt/blog-backend/internal/handler/user"
	"github.com/vpt/blog-backend/internal/middleware"
	oauthflow "github.com/vpt/blog-backend/internal/oauth"
	oauthproviders "github.com/vpt/blog-backend/internal/oauth/providers"
	analyticsrepo "github.com/vpt/blog-backend/internal/repository/analytics"
	articlerepo "github.com/vpt/blog-backend/internal/repository/article"
	categoryrepo "github.com/vpt/blog-backend/internal/repository/category"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	dashboardrepo "github.com/vpt/blog-backend/internal/repository/dashboard"
	friendlinkrepo "github.com/vpt/blog-backend/internal/repository/friendlink"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
	musicrepo "github.com/vpt/blog-backend/internal/repository/music"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
	socialauthrepo "github.com/vpt/blog-backend/internal/repository/socialauth"
	tagrepo "github.com/vpt/blog-backend/internal/repository/tag"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	analyticsservice "github.com/vpt/blog-backend/internal/service/analytics"
	articleservice "github.com/vpt/blog-backend/internal/service/article"
	authservice "github.com/vpt/blog-backend/internal/service/auth"
	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
	captchaservice "github.com/vpt/blog-backend/internal/service/captcha"
	categoryservice "github.com/vpt/blog-backend/internal/service/category"
	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	dashboardservice "github.com/vpt/blog-backend/internal/service/dashboard"
	friendlinkservice "github.com/vpt/blog-backend/internal/service/friendlink"
	guestbookservice "github.com/vpt/blog-backend/internal/service/guestbook"
	momentservice "github.com/vpt/blog-backend/internal/service/moment"
	musicservice "github.com/vpt/blog-backend/internal/service/music"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
	oauthservice "github.com/vpt/blog-backend/internal/service/oauth"
	tagservice "github.com/vpt/blog-backend/internal/service/tag"
	uploadservice "github.com/vpt/blog-backend/internal/service/upload"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/internal/service/uv"
	analyticsworker "github.com/vpt/blog-backend/internal/worker/analytics"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/email"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"time"
)

const corsAllowedOriginsEnv = "CORS_ALLOWED_ORIGINS"

type routeHandlers struct {
	health            *handler.HealthHandler
	test              *handler.TestHandler
	auth              *authhandler.AuthHandler
	oauth             *oauthhandler.OAuthHandler
	captcha           *captchahandler.CaptchaHandler
	article           *articlehandler.ArticleHandler
	comment           *commenthandler.CommentHandler
	guestbook         *guestbookhandler.GuestbookHandler
	moment            *momenthandler.MomentHandler
	notification      *notificationhandler.NotificationHandler
	notificationAdmin *notificationhandler.NotificationAdminHandler
	user              *userhandler.UserHandler
	userAdmin         *userhandler.UserAdminHandler
	category          *categoryhandler.CategoryHandler
	tag               *taghandler.TagHandler
	music             *musichandler.MusicHandler
	friendLink        *friendlinkhandler.FriendLinkHandler
	upload            *uploadhandler.Handler
	analyticsCollect  *analyticshandler.CollectHandler
	analyticsAdmin    *analyticshandler.AdminHandler
	analyticsPublic   *analyticshandler.PublicHandler
	analyticsRuntime  AnalyticsRuntime
	dashboard         *dashboardhandler.Handler
	userCache         userservice.UserCacheService
}

// AnalyticsRuntime 暴露统计上报链路中需要被 worker 复用的实例。
// 关键约束：collect handler 与聚合 worker 必须共享同一个 Ingestor —
// 生产者（collect）投递与消费者（worker.Run）必须作用于同一实例，否则事件只入队不落库。
type AnalyticsRuntime struct {
	Ingestor analyticsworker.Ingestor
	Repo     analyticsrepo.Repository
	TZ       *time.Location
}

// Setup 注册所有路由，是整个项目路由的唯一入口。
// 返回 AnalyticsRuntime，供 main 启动唯一的聚合/落库 worker。
func Setup(
	r *gin.Engine,
	log *zap.Logger,
	jwtManager *jwt.Manager,
	db *gorm.DB,
	redisClient *redis.Client,
	mailer email.MailSender,
	objectStore storage.ObjectStore,
	cfg *config.Config,
	notificationHub *notificationservice.SSEHub,
) AnalyticsRuntime {
	// 配置信任代理，确保反向代理链路下能拿到真实客户端 IP。
	configureTrustedProxies(r)

	// 注册跨域中间件，支持开发环境和生产代理环境的来源策略。
	r.Use(cors.New(newCORSConfig()))

	// 注册全局基础中间件，统一处理请求追踪、恢复和请求日志。
	r.Use(middleware.RequestID(), middleware.Recovery(log), middleware.Logger(log))

	// 组装路由所需的 handler，保持 Setup 只关心注册流程。
	handlers := newRouteHandlers(log, db, redisClient, jwtManager, mailer, objectStore, cfg, notificationHub)

	// 按权限层级注册路由，公开路由在前，受保护路由在后。
	registerPublicRoutes(r, handlers, jwtManager, redisClient)
	registerAuthedRoutes(r, handlers, jwtManager, redisClient)
	registerVIPRoutes(r, handlers, jwtManager)
	registerAdminRoutes(r, handlers, jwtManager, redisClient)

	return handlers.analyticsRuntime
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
	log *zap.Logger,
	db *gorm.DB,
	redisClient *redis.Client,
	jwtManager *jwt.Manager,
	mailer email.MailSender,
	objectStore storage.ObjectStore,
	cfg *config.Config,
	notificationHub *notificationservice.SSEHub,
) routeHandlers {
	// 组装图形验证码链路，注册发送邮箱验证码前会消费它签发的一次性票据。
	captchaSvc, err := captchaservice.NewService(redisClient)
	if err != nil {
		panic(err)
	}

	// 组装认证链路，保持依赖从 repository 到 service 再到 handler 的方向。
	userRepo := userrepo.NewUserRepository(db)
	userCacheSvc := userservice.NewUserCacheService(userRepo, objectStore, redisClient)
	onlineWindow := cfg.Analytics.OnlineWindow
	if onlineWindow <= 0 {
		onlineWindow = 90 * time.Second
	}
	userPresence := analyticsservice.NewUserPresence(redisClient, userRepo, userCacheSvc, onlineWindow)
	avatarSvc := avatarservice.NewService(objectStore, avatarservice.Options{})
	authSvc := authservice.NewAuthService(userRepo, jwtManager, redisClient, mailer, captchaSvc, userCacheSvc, avatarSvc, objectStore, userPresence)
	userSvc := userservice.NewUserService(userCacheSvc, userRepo, objectStore, avatarSvc, userPresence, userservice.SecurityDeps{
		Redis:   redisClient,
		Mailer:  mailer,
		Captcha: captchaSvc,
	})
	userAdminSvc := userservice.NewAdminService(userRepo, userCacheSvc)
	socialAuthRepo := socialauthrepo.NewSocialAuthRepository(db)
	oauthManager := newOAuthManager(redisClient, cfg)
	oauthSvc := oauthservice.NewOAuthService(oauthManager, socialAuthRepo, userRepo, jwtManager, userCacheSvc, avatarSvc, userPresence)

	uvSvc := uv.NewService(redisClient)

	// 组装通知链路：仓储 + 事件发布器（业务侧用），收件箱服务供站内接口。
	notificationRepo := notificationrepo.NewRepository(db)
	notificationDirectory := notificationrepo.NewDirectory(db)
	notificationPublisher := notificationservice.NewPublisher(notificationRepo)
	notificationInboxSvc := notificationservice.NewInboxService(notificationRepo, notificationDirectory, notificationDirectory, objectStore)
	notificationAdminSvc := notificationservice.NewAdminService(notificationrepo.NewAdminRepository(db))

	// 组装文章链路，前端对象地址由 service 层统一解析。
	articleRepo := articlerepo.NewArticleRepository(db)
	articleSvc := articleservice.NewArticleService(articleRepo, objectStore, uvSvc, notificationPublisher)

	categoryRepo := categoryrepo.NewCategoryRepository(db)
	categorySvc := categoryservice.NewCategoryService(categoryRepo)

	tagRepo := tagrepo.NewTagRepository(db)
	tagSvc := tagservice.NewTagService(tagRepo, articleSvc)

	musicRepo := musicrepo.NewMusicRepository(db)
	musicSvc := musicservice.NewMusicService(musicRepo, objectStore)

	friendLinkRepo := friendlinkrepo.NewFriendLinkRepository(db)
	friendLinkSvc := friendlinkservice.NewFriendLinkService(friendLinkRepo, objectStore)

	commentRepo := commentrepo.NewCommentRepository(db)
	commentSvc := commentservice.NewCommentService(commentRepo, objectStore, notificationPublisher, userRepo)

	guestbookRepo := guestbookrepo.NewGuestbookRepository(db)
	guestbookSvc := guestbookservice.NewGuestbookService(guestbookRepo, objectStore, notificationPublisher, userRepo)

	momentRepo := momentrepo.NewMomentRepository(db)
	momentSvc := momentservice.NewMomentService(momentRepo, objectStore, uvSvc, notificationPublisher, userRepo)
	uploadSvc := uploadservice.NewService(objectStore)

	// 组装站点统计上报链路：富化 → 实时层 → 异步落库 + 会话写入 → PV 去重。
	analyticsCollectHandler, analyticsAdminHandler, analyticsPublicHandler, analyticsRuntime := newAnalyticsCollectHandler(log, db, redisClient, uvSvc, cfg.Analytics, userPresence)

	// 后台首页汇总（内容总量/近期互动/用户统计），切天时区与统计口径一致。
	dashTZ, dashErr := time.LoadLocation(cfg.Analytics.Timezone)
	if dashErr != nil {
		dashTZ = time.FixedZone("CST", 8*3600)
	}
	dashboardHandler := dashboardhandler.NewHandler(
		dashboardservice.NewService(dashboardrepo.NewRepository(db), dashTZ),
	)

	return routeHandlers{
		health:            handler.NewHealthHandler(db, redisClient),
		test:              handler.NewTestHandler(jwtManager),
		auth:              authhandler.NewAuthHandler(authSvc),
		oauth:             oauthhandler.NewOAuthHandler(oauthSvc),
		captcha:           captchahandler.NewCaptchaHandler(captchaSvc),
		article:           articlehandler.NewArticleHandler(articleSvc),
		comment:           commenthandler.NewCommentHandler(commentSvc),
		guestbook:         guestbookhandler.NewGuestbookHandler(guestbookSvc),
		moment:            momenthandler.NewMomentHandler(momentSvc),
		notification:      notificationhandler.NewNotificationHandler(notificationInboxSvc, notificationHub),
		notificationAdmin: notificationhandler.NewNotificationAdminHandler(notificationAdminSvc),
		user:              userhandler.NewUserHandler(userSvc),
		userAdmin:         userhandler.NewUserAdminHandler(userAdminSvc, log),
		category:          categoryhandler.NewCategoryHandler(categorySvc),
		tag:               taghandler.NewTagHandler(tagSvc),
		music:             musichandler.NewMusicHandler(musicSvc),
		friendLink:        friendlinkhandler.NewFriendLinkHandler(friendLinkSvc),
		upload:            uploadhandler.NewHandler(uploadSvc),
		analyticsCollect:  analyticsCollectHandler,
		analyticsAdmin:    analyticsAdminHandler,
		analyticsPublic:   analyticsPublicHandler,
		analyticsRuntime:  analyticsRuntime,
		dashboard:         dashboardHandler,
		userCache:         userCacheSvc,
	}
}

const analyticsAllowedOriginsEnv = "ANALYTICS_ALLOWED_ORIGINS"

// newAnalyticsCollectHandler 组装 /collect 上报所需依赖图，并返回供 worker 复用的运行时实例。
// 这里构造的 ingestor 是「唯一」实例：collect handler 经 sessionIng 向它投递，
// worker.Run 也消费同一个，二者不可分裂为两个实例。
// 采集相关参数全部来自 cfg.Analytics（敏感值经 BLOG_ 前缀 env 覆盖）。
func newAnalyticsCollectHandler(
	log *zap.Logger,
	db *gorm.DB,
	redisClient *redis.Client,
	uvSvc uv.UVService,
	analyticsCfg config.AnalyticsConfig,
	userPresence analyticsservice.UserPresence,
) (*analyticshandler.CollectHandler, *analyticshandler.AdminHandler, *analyticshandler.PublicHandler, AnalyticsRuntime) {
	tz, err := time.LoadLocation(analyticsCfg.Timezone)
	if err != nil {
		// 与 repository 层 dayRangeUTC 兜底一致：时区解析失败回退东八区。
		log.Warn("统计时区解析失败，回退东八区", zap.String("timezone", analyticsCfg.Timezone), zap.Error(err))
		tz = time.FixedZone("CST", 8*3600)
	}

	geo := analyticsservice.NewGeoResolver(analyticsCfg.GeoIPV4Path, analyticsCfg.GeoIPV6Path, log)
	enricher := analyticsservice.NewEnricher(geo, analyticsCfg.SiteHost, analyticsCfg.IPSalt)
	analyticsRepo := analyticsrepo.NewRepository(db)
	realtime := analyticsservice.NewRealtime(redisClient, tz, analyticsCfg.OnlineWindow)
	ingestor := analyticsworker.NewIngestor(analyticsRepo, analyticsCfg.ChannelBuffer, 100, 2*time.Second, log)
	sessionIng := analyticsworker.NewSessionIngestor(ingestor, analyticsRepo)
	dedup := analyticsservice.NewDedupChecker(uvSvc)
	tokenVerifier := analyticsservice.NewCollectTokenVerifier(analyticsCfg.CollectTokenSecret, analyticsCfg.CollectTokenTTL, nil)
	collectSvc := analyticsservice.NewCollectService(enricher, realtime, sessionIng, dedup, tokenVerifier, userPresence, log)

	// 后台只读查询复用同一 repo（历史累计）与 realtime（今日/在线），不另起依赖图。
	querySvc := analyticsservice.NewQueryService(analyticsRepo, realtime, log)
	// 专用的回填 Rollup：RollupDay 幂等且无状态，可与调度器实例分开。
	backfillRollup := analyticsworker.NewRollup(analyticsRepo, analyticsRepo, log)
	backfillSvc := analyticsservice.NewBackfillService(backfillRollup.RollupDay)
	adminHandler := analyticshandler.NewAdminHandler(querySvc, backfillSvc)

	// 前台公开统计：复用同一 repo/realtime，响应 JSON 走 Redis 短 TTL 缓存。
	publicSvc := analyticsservice.NewPublicService(analyticsRepo, realtime, redisClient, analyticsCfg.PublicCacheTTL, log)
	publicHandler := analyticshandler.NewPublicHandler(publicSvc)

	allowedOrigins := splitCORSOrigins(os.Getenv(analyticsAllowedOriginsEnv))
	runtime := AnalyticsRuntime{Ingestor: ingestor, Repo: analyticsRepo, TZ: tz}
	return analyticshandler.NewCollectHandler(collectSvc, allowedOrigins), adminHandler, publicHandler, runtime
}

func newOAuthManager(redisClient *redis.Client, cfg *config.Config) *oauthflow.Manager {
	// state TTL 使用配置值；未配置时 StateStore 内部会回落到 10 分钟。
	var ttl time.Duration
	if cfg != nil && cfg.OAuth.StateTTLMinutes > 0 {
		ttl = time.Duration(cfg.OAuth.StateTTLMinutes) * time.Minute
	}

	providers := make([]oauthflow.Provider, 0)
	if cfg != nil {
		if githubCfg, ok := cfg.OAuth.Providers["github"]; ok && githubCfg.Enabled {
			providers = append(providers, oauthproviders.NewGitHubProvider(githubCfg))
		}
		if giteeCfg, ok := cfg.OAuth.Providers["gitee"]; ok && giteeCfg.Enabled {
			providers = append(providers, oauthproviders.NewGiteeProvider(giteeCfg))
		}
		if qqCfg, ok := cfg.OAuth.Providers["qq"]; ok && qqCfg.Enabled {
			providers = append(providers, oauthproviders.NewQQProvider(qqCfg))
		}
		if weiboCfg, ok := cfg.OAuth.Providers["weibo"]; ok && weiboCfg.Enabled {
			providers = append(providers, oauthproviders.NewWeiboProvider(weiboCfg))
		}
		if baiduCfg, ok := cfg.OAuth.Providers["baidu"]; ok && baiduCfg.Enabled {
			providers = append(providers, oauthproviders.NewBaiduProvider(baiduCfg))
		}
	}

	return oauthflow.NewManager(oauthflow.NewStateStore(redisClient, ttl), providers)
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
	r.POST("/auth/password-reset/code", middleware.RateLimitStrict(redisClient), handlers.auth.SendPasswordResetCode)
	r.POST("/auth/password-reset", middleware.RateLimitStrict(redisClient), handlers.auth.ResetPassword)
	r.POST("/auth/register", middleware.RateLimitStrict(redisClient), handlers.auth.Register)
	r.POST("/auth/login", middleware.RateLimitNormal(redisClient), handlers.auth.Login)
	r.POST("/admin/auth/login", middleware.RateLimitNormal(redisClient), handlers.auth.AdminLogin)
	r.POST("/auth/refresh", handlers.auth.Refresh)
	r.GET("/oauth/providers", handlers.oauth.Providers)
	r.GET("/oauth/:source/authorize", middleware.RateLimitNormal(redisClient), middleware.OptionalAuth(jwtManager), handlers.oauth.Authorize)
	r.GET("/oauth/:source/callback", middleware.RateLimitNormal(redisClient), handlers.oauth.Callback)
	r.GET("/categories", handlers.category.ListTabs)
	r.GET("/tags", handlers.tag.List)
	r.GET("/tags/:id", handlers.tag.Get)
	r.GET("/tags/:id/articles", handlers.tag.ListArticles)
	r.GET("/music", handlers.music.List)
	r.GET("/music/artists", handlers.music.ListArtists)
	r.GET("/music/artists/:id", handlers.music.GetPublicArtist)
	r.GET("/music/albums", handlers.music.ListAlbums)
	r.GET("/music/albums/:id", handlers.music.GetPublicAlbum)
	r.GET("/music/:id", handlers.music.GetPublicDetail)
	r.GET("/friend-links", handlers.friendLink.ListPublic)
	r.GET("/friend-links/:id", handlers.friendLink.GetPublic)
	r.GET("/users", handlers.user.ListAll)
	r.GET("/users/recent", handlers.user.ListRecent)
	r.GET("/users/:id/likes/count", handlers.user.CountLikedContent)
	r.GET("/users/:id/likes", handlers.user.ListLikedContent)
	r.GET("/users/:id", middleware.OptionalAuth(jwtManager), handlers.user.GetPublicProfile)
	r.GET("/articles/ids", handlers.article.ListIDs)
	r.GET("/articles", middleware.OptionalAuth(jwtManager), handlers.article.ListPublic)
	r.GET("/articles/:id", middleware.OptionalAuth(jwtManager), handlers.article.GetPublicDetail)
	r.POST("/articles/:id/view", middleware.VisitorID(), handlers.article.View)
	r.GET("/articles/:id/comments", middleware.OptionalAuth(jwtManager), handlers.comment.ListArticle)
	r.GET("/guestbook", middleware.OptionalAuth(jwtManager), handlers.guestbook.List)
	r.GET("/guestbook/comments/:id/replies", middleware.OptionalAuth(jwtManager), handlers.comment.ListGuestbookReplies)
	r.GET("/moments", middleware.OptionalAuth(jwtManager), handlers.moment.List)
	r.GET("/moments/feed", middleware.OptionalAuth(jwtManager), handlers.moment.Feed)
	r.GET("/moments/:id", middleware.OptionalAuth(jwtManager), handlers.moment.GetDetail)
	r.GET("/moments/:id/comments", middleware.OptionalAuth(jwtManager), handlers.comment.ListMoment)
	r.GET("/moments/comments/:id/replies", middleware.OptionalAuth(jwtManager), handlers.comment.ListMomentReplies)
	r.GET("/articles/comments/:id/replies", middleware.OptionalAuth(jwtManager), handlers.comment.ListArticleReplies)
	r.POST("/moments/:id/view", middleware.VisitorID(), handlers.moment.View)
	r.POST("/collect",
		middleware.VisitorID(),
		middleware.OptionalAuth(jwtManager),
		middleware.RateLimitNormal(redisClient),
		handlers.analyticsCollect.Collect,
	)
	r.GET("/analytics/public/summary", middleware.RateLimitNormal(redisClient), handlers.analyticsPublic.Summary)
	r.GET("/analytics/public/popular", middleware.RateLimitNormal(redisClient), handlers.analyticsPublic.Popular)
}

func registerAuthedRoutes(r *gin.Engine, handlers routeHandlers, jwtManager *jwt.Manager, redisClient *redis.Client) {
	// 登录路由要求任意已认证用户。
	authed := r.Group("/", middleware.Auth(jwtManager, handlers.userCache))
	authed.GET("/test/authed", handlers.test.Authed)
	authed.GET("/users/me", handlers.user.GetDetail)
	authed.PUT("/users/me", handlers.user.Update)
	authed.POST("/users/me/avatar", middleware.RateLimitAvatarUpload(redisClient), handlers.user.UploadAvatar)
	authed.PATCH("/users/me/profile", handlers.user.UpdateProfile)
	authed.PATCH("/users/me/meta", handlers.user.UpdateMeta)
	authed.PATCH("/users/me/social/:platform", handlers.user.UpdateSocialLink)
	authed.PATCH("/users/me/username", handlers.user.UpdateUsername)
	authed.PATCH("/users/me/password", handlers.user.UpdatePassword)
	authed.PATCH("/users/me/password/initial", handlers.user.SetInitialPassword)
	authed.POST("/users/me/email/code", middleware.RateLimitStrict(redisClient), handlers.user.SendEmailCode)
	authed.PATCH("/users/me/email", handlers.user.UpdateEmail)
	authed.PATCH("/users/me/email/display", handlers.user.UpdateEmailDisplay)
	authed.GET("/users/me/oauth-bindings", handlers.oauth.ListBindings)
	authed.GET("/oauth/bindings", handlers.oauth.ListBindings)
	authed.DELETE("/oauth/bindings/:source", handlers.oauth.Unbind)
	authed.POST("/uploads/temp", middleware.RateLimitTempUpload(redisClient), handlers.upload.TempImage)
	authed.GET("/articles/:id/like", handlers.article.IsLiked)
	authed.POST("/articles/:id/like", handlers.article.ToggleLike)
	authed.POST("/articles/:id/comments", handlers.comment.CreateArticle)
	authed.POST("/articles/comments/:id/replies", handlers.comment.ReplyArticle)
	authed.POST("/articles/comments/:id/like", handlers.comment.ToggleArticleLike)
	authed.POST("/articles/comments/:id/replies/:replyId/like", handlers.comment.ToggleArticleReplyLike)
	authed.DELETE("/articles/comments/:id", handlers.comment.DeleteArticle)
	authed.DELETE("/articles/comment-replies/:id", handlers.comment.DeleteArticleReply)
	authed.POST("/guestbook", handlers.guestbook.Create)
	authed.POST("/guestbook/:id/like", handlers.guestbook.ToggleLike)
	authed.POST("/guestbook/comments/:id/replies", handlers.comment.ReplyGuestbook)
	authed.POST("/guestbook/comments/:id/replies/:replyId/like", handlers.comment.ToggleGuestbookReplyLike)
	authed.DELETE("/guestbook/comments/:id", handlers.comment.DeleteGuestbook)
	authed.DELETE("/guestbook/comment-replies/:id", handlers.comment.DeleteGuestbookReply)
	authed.DELETE("/guestbook/:id", handlers.guestbook.Delete)
	authed.POST("/moments", middleware.RateLimitMomentUpload(redisClient), handlers.moment.Save)
	authed.POST("/moments/:id/comments", handlers.comment.CreateMoment)
	authed.POST("/moments/comments/:id/replies", handlers.comment.ReplyMoment)
	authed.POST("/moments/comments/:id/like", handlers.comment.ToggleMomentLike)
	authed.POST("/moments/comments/:id/replies/:replyId/like", handlers.comment.ToggleMomentReplyLike)
	authed.DELETE("/moments/comments/:id", handlers.comment.DeleteMoment)
	authed.DELETE("/moments/comment-replies/:id", handlers.comment.DeleteMomentReply)
	authed.DELETE("/moments/:id", handlers.moment.Delete)
	authed.POST("/moments/:id/top", handlers.moment.SetTop)
	authed.DELETE("/moments/:id/top", handlers.moment.RemoveTop)
	authed.GET("/moments/:id/like", handlers.moment.IsLiked)
	authed.POST("/moments/:id/like", handlers.moment.ToggleLike)
	authed.GET("/notifications", handlers.notification.List)
	authed.GET("/notifications/unread-count", handlers.notification.UnreadCount)
	authed.PATCH("/notifications/read", handlers.notification.MarkAllRead)
	authed.PATCH("/notifications/:id/read", handlers.notification.MarkRead)
	authed.DELETE("/notifications/:id", handlers.notification.Delete)
	authed.GET("/notifications/stream", handlers.notification.Stream)
}

func registerVIPRoutes(r *gin.Engine, handlers routeHandlers, jwtManager *jwt.Manager) {
	// VIP 路由要求 VIP 或更高权限。
	vip := r.Group("/", middleware.Auth(jwtManager, handlers.userCache), middleware.RequireRole(roles.VipRole))
	vip.GET("/test/vip", handlers.test.Vip)
}

func registerAdminRoutes(r *gin.Engine, handlers routeHandlers, jwtManager *jwt.Manager, redisClient *redis.Client) {
	// 管理员路由统一挂在 /admin 前缀下。
	admin := r.Group("/admin", middleware.Auth(jwtManager, handlers.userCache), middleware.RequireRole(roles.AdminRole))
	admin.GET("/test", handlers.test.Admin)
	admin.GET("/articles", handlers.article.ListAdmin)
	admin.GET("/articles/:id", handlers.article.GetAdminDetail)
	admin.POST("/articles", handlers.article.Save)
	admin.DELETE("/articles/:id", handlers.article.Delete)
	admin.DELETE("/articles/:id/permanent", handlers.article.PermanentDelete)
	admin.GET("/comments", handlers.comment.ListAdmin)
	admin.GET("/guestbook", handlers.guestbook.ListAdmin)
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
	admin.GET("/music/artists", handlers.music.ListAdminArtists)
	admin.POST("/music/artists", handlers.music.SaveArtist)
	admin.PUT("/music/artists/:id", handlers.music.SaveArtist)
	admin.DELETE("/music/artists/:id", handlers.music.DeleteArtist)
	admin.GET("/music/albums", handlers.music.ListAdminAlbums)
	admin.POST("/music/albums", handlers.music.SaveAlbum)
	admin.PUT("/music/albums/:id", handlers.music.SaveAlbum)
	admin.DELETE("/music/albums/:id", handlers.music.DeleteAlbum)
	admin.GET("/music", handlers.music.ListAdmin)
	admin.POST("/music", handlers.music.SaveMusic)
	admin.PUT("/music/:id", handlers.music.SaveMusic)
	admin.DELETE("/music/:id", handlers.music.DeleteMusic)
	admin.POST("/music/uploads/audio", handlers.music.UploadAudio)
	admin.POST("/music/uploads/album-cover", handlers.music.UploadAlbumCover)
	admin.POST("/music/uploads/artist-avatar", handlers.music.UploadArtistAvatar)
	admin.GET("/friend-links", handlers.friendLink.ListAdmin)
	admin.POST("/friend-links", middleware.RateLimitTempUpload(redisClient), handlers.friendLink.Create)
	admin.PUT("/friend-links/:id", middleware.RateLimitTempUpload(redisClient), handlers.friendLink.Update)
	admin.DELETE("/friend-links/:id", handlers.friendLink.Delete)
	admin.GET("/notifications/email-tasks", handlers.notificationAdmin.ListEmailTasks)
	admin.GET("/notifications/email-batches", handlers.notificationAdmin.ListEmailBatches)
	admin.GET("/notifications/email-quotas", handlers.notificationAdmin.ListQuotas)
	admin.PUT("/notifications/email-quotas/:id", handlers.notificationAdmin.UpdateQuota)
	admin.PUT("/notifications/role-quotas/:id", handlers.notificationAdmin.UpdateRoleQuota)
	admin.POST("/notifications/email-batches/:id/retry", handlers.notificationAdmin.RetryBatch)
	admin.POST("/users/:id/roles/vip", handlers.userAdmin.GrantVip)
	admin.DELETE("/users/:id/roles/vip", handlers.userAdmin.RevokeVip)
	admin.GET("/overview/summary", handlers.dashboard.Overview)
	admin.GET("/analytics/overview", handlers.analyticsAdmin.Overview)
	admin.GET("/analytics/trend", handlers.analyticsAdmin.Trend)
	admin.GET("/analytics/pages", handlers.analyticsAdmin.Pages)
	admin.GET("/analytics/dimensions", handlers.analyticsAdmin.Dimensions)
	admin.GET("/analytics/friend-links", handlers.analyticsAdmin.FriendLinks)
	admin.GET("/analytics/paths", handlers.analyticsAdmin.Paths)
	admin.GET("/analytics/funnel", handlers.analyticsAdmin.Funnel)
	admin.GET("/analytics/realtime", handlers.analyticsAdmin.Realtime)
	admin.POST("/analytics/backfill", middleware.RateLimitNormal(redisClient), handlers.analyticsAdmin.Backfill)
}
