package router

import (
	"context"
	"os"
	"time"

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
	oauthflow "github.com/vpt/blog-backend/internal/oauth"
	oauthproviders "github.com/vpt/blog-backend/internal/oauth/providers"
	adminlogrepo "github.com/vpt/blog-backend/internal/repository/adminlog"
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
	adminlogservice "github.com/vpt/blog-backend/internal/service/adminlog"
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
	moderationruleworker "github.com/vpt/blog-backend/internal/service/moderationrule"
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
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type routeHandlers struct {
	health               *handler.HealthHandler
	test                 *handler.TestHandler
	auth                 *authhandler.AuthHandler
	oauth                *oauthhandler.OAuthHandler
	captcha              *captchahandler.CaptchaHandler
	article              *articlehandler.ArticleHandler
	comment              *commenthandler.CommentHandler
	guestbook            *guestbookhandler.GuestbookHandler
	moment               *momenthandler.MomentHandler
	moderationAdmin      *moderationhandler.AdminHandler
	notification         *notificationhandler.NotificationHandler
	notificationAdmin    *notificationhandler.NotificationAdminHandler
	user                 *userhandler.UserHandler
	userAdmin            *userhandler.UserAdminHandler
	category             *categoryhandler.CategoryHandler
	tag                  *taghandler.TagHandler
	music                *musichandler.MusicHandler
	friendLink           *friendlinkhandler.FriendLinkHandler
	upload               *uploadhandler.Handler
	analyticsCollect     *analyticshandler.CollectHandler
	analyticsAdmin       *analyticshandler.AdminHandler
	analyticsPublic      *analyticshandler.PublicHandler
	analyticsRuntime     AnalyticsRuntime
	moderationRuleWorker moderationruleworker.Worker
	dashboard            *dashboardhandler.Handler
	userCache            userservice.UserCacheService
	moderationRateLimit  config.ModerationRateLimitConfig
	runtime              Runtime
}

// AnalyticsRuntime 暴露统计上报链路中需要被 worker 复用的实例。
// 关键约束：collect handler 与聚合 worker 必须共享同一个 Ingestor —
// 生产者（collect）投递与消费者（worker.Run）必须作用于同一实例，否则事件只入队不落库。
type AnalyticsRuntime struct {
	Ingestor analyticsworker.Ingestor
	Repo     analyticsrepo.Repository
	TZ       *time.Location
}

// Runtime 是 router.Setup 返回的复合运行时，供 main 启动后台 worker。
type Runtime struct {
	Analytics       AnalyticsRuntime
	ModerationRules moderationruleworker.Worker
}

// Setup 注册所有路由，是整个项目路由的唯一入口。
// 返回 Runtime，供 main 启动统计聚合 worker 和规则构建 worker。
func Setup(
	r *gin.Engine,
	log *zap.Logger,
	jwtManager *jwt.Manager,
	db *gorm.DB,
	redisClient *redis.Client,
	mailer email.MailSender,
	objectStore storage.ObjectStore,
	cfg *config.Config,
) Runtime {
	// 配置信任代理，确保反向代理链路下能拿到真实客户端 IP。
	configureTrustedProxies(r)

	// 注册跨域中间件，支持开发环境和生产代理环境的来源策略。
	r.Use(cors.New(newCORSConfig()))

	// 注册全局基础中间件，统一处理请求追踪、恢复和请求日志。
	r.Use(middleware.RequestID(), middleware.Recovery(log), middleware.Logger(log))

	registerCDNImageRoutes(r, objectStore, cfg, log)

	// 组装路由所需的 handler，保持 Setup 只关心注册流程。
	handlers := newRouteHandlers(log, db, redisClient, jwtManager, mailer, objectStore, cfg)

	// 按权限层级注册路由，公开路由在前，受保护路由在后。
	registerPublicRoutes(r, handlers, jwtManager, redisClient)
	registerAuthedRoutes(r, handlers, jwtManager, redisClient)
	registerVIPRoutes(r, handlers, jwtManager)
	registerAdminRoutes(r, handlers, jwtManager, redisClient)

	return handlers.runtime
}

func newRouteHandlers(
	log *zap.Logger,
	db *gorm.DB,
	redisClient *redis.Client,
	jwtManager *jwt.Manager,
	mailer email.MailSender,
	objectStore storage.ObjectStore,
	cfg *config.Config,
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
	moderationGovernanceSvc := maybeNewModerationGovernanceService(db, cfg.Moderation)
	authSvc := authservice.NewAuthService(userRepo, jwtManager, redisClient, mailer, captchaSvc, userCacheSvc, avatarSvc, objectStore, userPresence, moderationGovernanceSvc)
	userSvc := userservice.NewUserService(userCacheSvc, userRepo, objectStore, avatarSvc, userPresence, userservice.SecurityDeps{
		Redis:   redisClient,
		Mailer:  mailer,
		Captcha: captchaSvc,
	})
	presenceProvider := userservice.NewPresenceProvider(userPresence, userRepo)
	friendLinkRepo := friendlinkrepo.NewFriendLinkRepository(db)
	adminlogRepo := adminlogrepo.NewRepository(db)
	adminlogSvc := adminlogservice.NewService(adminlogRepo)
	userAdminSvc := userservice.NewAdminService(userRepo, userCacheSvc, userservice.AdminDeps{
		Store:      objectStore,
		Avatar:     avatarSvc,
		FriendLink: friendLinkRepo,
		Moderation: newModerationProfileReader(moderationGovernanceSvc),
		Presence:   userPresence,
		Logs:       adminlogRepo,
	})
	socialAuthRepo := socialauthrepo.NewSocialAuthRepository(db)
	oauthManager := newOAuthManager(redisClient, cfg)
	oauthSvc := oauthservice.NewOAuthService(oauthManager, socialAuthRepo, userRepo, jwtManager, userCacheSvc, avatarSvc, userPresence, moderationGovernanceSvc)

	uvSvc := uv.NewService(redisClient)

	// 组装通知链路：仓储 + 事件发布器（业务侧用），收件箱服务供站内接口。
	notificationRepo := notificationrepo.NewRepository(db)
	notificationDirectory := notificationrepo.NewDirectory(db)
	notificationPublisher := notificationservice.NewPublisher(notificationRepo)
	notificationInboxSvc := notificationservice.NewInboxService(notificationRepo, notificationDirectory, notificationDirectory, objectStore)
	notificationAdminSvc := notificationservice.NewAdminService(notificationrepo.NewAdminRepository(db))

	// 组装文章链路，前端对象地址由 service 层统一解析。
	articleRepo := articlerepo.NewArticleRepository(db)
	musicRepo := musicrepo.NewMusicRepository(db)
	musicSvc := musicservice.NewMusicService(musicRepo, objectStore)
	articleSvc := articleservice.NewArticleService(articleRepo, objectStore, uvSvc, notificationPublisher, musicSvc)

	categoryRepo := categoryrepo.NewCategoryRepository(db)
	categorySvc := categoryservice.NewCategoryService(categoryRepo, objectStore, log)

	tagRepo := tagrepo.NewTagRepository(db)
	tagSvc := tagservice.NewTagService(tagRepo, articleSvc)

	friendLinkSvc := friendlinkservice.NewFriendLinkService(friendLinkRepo, objectStore)

	moderationRuntime, moderationErr := maybeNewModerationService(context.Background(), db, cfg.Moderation, log, objectStore)
	if moderationErr != nil {
		panic(moderationErr)
	}
	moderationSvc := moderationRuntime.service
	moderationReviewSvc := maybeNewModerationReviewService(db, cfg.Moderation, log, objectStore)
	moderationOperationsSvc := maybeNewModerationOperationsService(db, cfg.Moderation, moderationGovernanceSvc)
	commentRepo := commentrepo.NewCommentRepository(db)
	commentSvc := commentservice.NewCommentService(commentRepo, objectStore, notificationPublisher, userRepo, moderationSvc)
	guestbookRepo := guestbookrepo.NewGuestbookRepository(db)
	guestbookSvc := guestbookservice.NewGuestbookService(guestbookRepo, objectStore, notificationPublisher, userRepo, moderationSvc)
	momentRepo := momentrepo.NewMomentRepository(db, cfg.Moderation.Enabled)
	momentSvc := momentservice.NewMomentService(momentRepo, objectStore, uvSvc, notificationPublisher, userRepo, moderationSvc)
	uploadSvc := uploadservice.NewService(objectStore, moderationGovernanceSvc)

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
		health:              handler.NewHealthHandler(db, redisClient),
		test:                handler.NewTestHandler(jwtManager),
		auth:                authhandler.NewAuthHandler(authSvc),
		oauth:               oauthhandler.NewOAuthHandler(oauthSvc),
		captcha:             captchahandler.NewCaptchaHandler(captchaSvc),
		article:             articlehandler.NewArticleHandler(articleSvc),
		comment:             commenthandler.NewCommentHandler(commentSvc, cfg.Moderation.Enabled),
		guestbook:           guestbookhandler.NewGuestbookHandler(guestbookSvc, cfg.Moderation.Enabled),
		moment:              momenthandler.NewMomentHandler(momentSvc, cfg.Moderation.Enabled),
		moderationAdmin:     newModerationAdminHandler(moderationReviewSvc, userCacheSvc, moderationOperationsSvc, cfg.Moderation.Rules.MaxImportFileMB, objectStore, adminlogSvc, moderationRuntime.ruleSvc),
		notification:        notificationhandler.NewNotificationHandler(notificationInboxSvc),
		notificationAdmin:   notificationhandler.NewNotificationAdminHandler(notificationAdminSvc),
		user:                userhandler.NewUserHandler(userSvc, momentSvc, presenceProvider),
		userAdmin:           userhandler.NewUserAdminHandler(userAdminSvc, log, adminlogSvc),
		category:            categoryhandler.NewCategoryHandler(categorySvc),
		tag:                 taghandler.NewTagHandler(tagSvc),
		music:               musichandler.NewMusicHandler(musicSvc),
		friendLink:          friendlinkhandler.NewFriendLinkHandler(friendLinkSvc),
		upload:              uploadhandler.NewHandler(uploadSvc),
		analyticsCollect:    analyticsCollectHandler,
		analyticsAdmin:      analyticsAdminHandler,
		analyticsPublic:     analyticsPublicHandler,
		analyticsRuntime:    analyticsRuntime,
		dashboard:           dashboardHandler,
		userCache:           userCacheSvc,
		moderationRateLimit: cfg.Moderation.RateLimit,
		runtime: Runtime{
			Analytics:       analyticsRuntime,
			ModerationRules: moderationRuntime.ruleWorker,
		},
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
