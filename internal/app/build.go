package app

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/handler"
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
	adminlogrepo "github.com/vpt/blog-backend/internal/repository/adminlog"
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
	"github.com/vpt/blog-backend/internal/router"
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
)

func buildHandlers(
	ctx context.Context,
	deps Dependencies,
) (router.Handlers, analyticsRuntime, moderationruleworker.Worker, error) {
	if deps.Config == nil || deps.Logger == nil || deps.DB == nil || deps.Redis == nil ||
		deps.JWT == nil || deps.Mailer == nil || deps.Store == nil {
		return router.Handlers{}, analyticsRuntime{}, nil, fmt.Errorf("应用基础设施依赖不完整")
	}
	cfg := deps.Config

	// 认证与用户链路共享同一仓储、缓存、在线状态和头像服务。
	captchaSvc, err := captchaservice.NewService(deps.Redis)
	if err != nil {
		return router.Handlers{}, analyticsRuntime{}, nil, fmt.Errorf("构造验证码服务: %w", err)
	}
	userRepo := userrepo.NewUserRepository(deps.DB)
	authRepo := userrepo.NewAuthenticationRepository(deps.DB)
	userCacheSvc := userservice.NewUserCacheService(userRepo, deps.Store, deps.Redis)
	onlineWindow := cfg.Analytics.OnlineWindow
	if onlineWindow <= 0 {
		onlineWindow = 90 * time.Second
	}
	userPresence := analyticsservice.NewUserPresence(deps.Redis, userRepo, userCacheSvc, onlineWindow)
	avatarSvc := avatarservice.NewService(deps.Store, avatarservice.Options{})
	moderationGovernanceSvc := maybeNewModerationGovernanceService(deps.DB, cfg.Moderation)
	authSvc := authservice.NewAuthService(
		authRepo, deps.JWT, deps.Redis, deps.Mailer, captchaSvc, userCacheSvc,
		avatarSvc, deps.Store, userPresence, moderationGovernanceSvc,
	)
	userSvc := userservice.NewUserService(userCacheSvc, userRepo, deps.Store, avatarSvc, userPresence, userservice.SecurityDeps{
		Redis: deps.Redis, Mailer: deps.Mailer, Captcha: captchaSvc,
	})
	presenceProvider := userservice.NewPresenceProvider(userPresence, userRepo)

	// 后台用户管理复用用户与友情链接仓储，并统一记录管理操作日志。
	friendLinkRepo := friendlinkrepo.NewFriendLinkRepository(deps.DB)
	adminlogRepo := adminlogrepo.NewRepository(deps.DB)
	adminlogSvc := adminlogservice.NewService(adminlogRepo)
	userAdminSvc := userservice.NewAdminService(userRepo, userCacheSvc, userservice.AdminDeps{
		Store: deps.Store, Avatar: avatarSvc, FriendLink: friendLinkRepo,
		Moderation: newModerationProfileReader(moderationGovernanceSvc), Presence: userPresence, Logs: adminlogRepo,
	})

	// OAuth 与账号体系共享用户仓储、缓存和治理入口。
	socialAuthRepo := socialauthrepo.NewSocialAuthRepository(deps.DB)
	oauthSvc := oauthservice.NewOAuthService(
		newOAuthManager(deps.Redis, cfg), socialAuthRepo, userRepo, deps.JWT,
		userCacheSvc, avatarSvc, userPresence, moderationGovernanceSvc,
	)
	uvSvc := uv.NewService(deps.Redis)

	// 通知发布器供内容模块写事件，收件箱与管理端使用各自最小服务。
	notificationRepo := notificationrepo.NewRepository(deps.DB)
	notificationDirectory := notificationrepo.NewDirectory(deps.DB)
	notificationPublisher := notificationservice.NewPublisher(notificationRepo)
	notificationInboxSvc := notificationservice.NewInboxService(
		notificationRepo, notificationDirectory, notificationDirectory, deps.Store,
	)
	notificationAdminSvc := notificationservice.NewAdminService(notificationrepo.NewAdminRepository(deps.DB))

	// 内容模块按仓储、服务顺序组装，共享对象存储、通知和审核能力。
	articleRepo := articlerepo.NewArticleRepository(deps.DB)
	musicRepo := musicrepo.NewMusicRepository(deps.DB)
	musicSvc := musicservice.NewMusicService(musicRepo, deps.Store)
	articleSvc := articleservice.NewArticleService(articleRepo, deps.Store, uvSvc, notificationPublisher, musicSvc)
	categorySvc := categoryservice.NewCategoryService(categoryrepo.NewCategoryRepository(deps.DB), deps.Store, deps.Logger)
	tagSvc := tagservice.NewTagService(tagrepo.NewTagRepository(deps.DB), articleSvc)
	friendLinkSvc := friendlinkservice.NewFriendLinkService(friendLinkRepo, deps.Store)

	moderationRuntime, err := maybeNewModerationService(ctx, deps.DB, cfg.Moderation, deps.Logger, deps.Store)
	if err != nil {
		return router.Handlers{}, analyticsRuntime{}, nil, fmt.Errorf("构造内容审核服务: %w", err)
	}
	moderationReviewSvc := maybeNewModerationReviewService(deps.DB, cfg.Moderation, deps.Logger, deps.Store)
	moderationOperationsSvc := maybeNewModerationOperationsService(deps.DB, cfg.Moderation, moderationGovernanceSvc)
	commentSvc := commentservice.NewCommentService(
		commentrepo.NewCommentRepository(deps.DB), deps.Store, notificationPublisher, userRepo, moderationRuntime.service,
	)
	guestbookSvc := guestbookservice.NewGuestbookService(
		guestbookrepo.NewGuestbookRepository(deps.DB), deps.Store, notificationPublisher, userRepo, moderationRuntime.service,
	)
	momentSvc := momentservice.NewMomentService(
		momentrepo.NewMomentRepository(deps.DB, cfg.Moderation.Enabled), deps.Store,
		uvSvc, notificationPublisher, userRepo, moderationRuntime.service,
	)
	uploadSvc := uploadservice.NewService(deps.Store, moderationGovernanceSvc)

	analyticsCollect, analyticsAdmin, analyticsPublic, analytics := newAnalyticsHandlers(
		deps.Logger, deps.DB, deps.Redis, uvSvc, cfg.Analytics, userPresence,
	)
	dashboardTZ, err := time.LoadLocation(cfg.Analytics.Timezone)
	if err != nil {
		dashboardTZ = time.FixedZone("CST", 8*3600)
	}

	handlers := router.Handlers{
		Health:            handler.NewHealthHandler(deps.DB, deps.Redis),
		Test:              handler.NewTestHandler(deps.JWT),
		Auth:              authhandler.NewAuthHandler(authSvc),
		OAuth:             oauthhandler.NewOAuthHandler(oauthSvc),
		Captcha:           captchahandler.NewCaptchaHandler(captchaSvc),
		Article:           articlehandler.NewArticleHandler(articleSvc),
		Comment:           commenthandler.NewCommentHandler(commentSvc, cfg.Moderation.Enabled),
		Guestbook:         guestbookhandler.NewGuestbookHandler(guestbookSvc, cfg.Moderation.Enabled),
		Moment:            momenthandler.NewMomentHandler(momentSvc, cfg.Moderation.Enabled),
		ModerationAdmin:   newModerationAdminHandler(moderationReviewSvc, userCacheSvc, moderationOperationsSvc, cfg.Moderation.Rules.MaxImportFileMB, deps.Store, adminlogSvc, moderationRuntime.ruleSvc),
		Notification:      notificationhandler.NewNotificationHandler(notificationInboxSvc),
		NotificationAdmin: notificationhandler.NewNotificationAdminHandler(notificationAdminSvc),
		User:              userhandler.NewUserHandler(userSvc, momentSvc, presenceProvider),
		UserAdmin:         userhandler.NewUserAdminHandler(userAdminSvc, deps.Logger, adminlogSvc),
		Category:          categoryhandler.NewCategoryHandler(categorySvc),
		Tag:               taghandler.NewTagHandler(tagSvc),
		Music:             musichandler.NewMusicHandler(musicSvc),
		FriendLink:        friendlinkhandler.NewFriendLinkHandler(friendLinkSvc),
		Upload:            uploadhandler.NewHandler(uploadSvc),
		AnalyticsCollect:  analyticsCollect,
		AnalyticsAdmin:    analyticsAdmin,
		AnalyticsPublic:   analyticsPublic,
		Dashboard: dashboardhandler.NewHandler(
			dashboardservice.NewService(dashboardrepo.NewRepository(deps.DB), dashboardTZ),
		),
		UserCache:           userCacheSvc,
		ModerationRateLimit: cfg.Moderation.RateLimit,
		CDNImage:            newCDNImageRoute(deps.Store, cfg, deps.Logger),
	}
	return handlers, analytics, moderationRuntime.ruleWorker, nil
}
