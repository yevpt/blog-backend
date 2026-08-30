// Package app 负责组装应用依赖、注册 HTTP 路由并启动后台任务。
package app

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/internal/bootstrap"
	analyticsrepo "github.com/vpt/blog-backend/internal/repository/analytics"
	"github.com/vpt/blog-backend/internal/router"
	moderationruleworker "github.com/vpt/blog-backend/internal/service/moderationrule"
	analyticsworker "github.com/vpt/blog-backend/internal/worker/analytics"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/email"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Dependencies 是应用组合根使用的基础设施依赖。
type Dependencies struct {
	Config *config.Config
	Logger *zap.Logger
	DB     *gorm.DB
	Redis  *redis.Client
	JWT    *jwt.Manager
	Mailer email.MailSender
	Store  storage.ObjectStore
}

type analyticsRuntime struct {
	Ingestor analyticsworker.Ingestor
	Repo     analyticsrepo.Repository
	TZ       *time.Location
}

// Application 持有已完成装配的 HTTP Handler 与后台运行时。
type Application struct {
	deps            Dependencies
	handlers        router.Handlers
	analytics       analyticsRuntime
	moderationRules moderationruleworker.Worker
}

// Build 按 repository → service → handler 顺序构造完整应用。
func Build(ctx context.Context, deps Dependencies) (*Application, error) {
	handlers, analytics, moderationRules, err := buildHandlers(ctx, deps)
	if err != nil {
		return nil, err
	}
	return &Application{
		deps: deps, handlers: handlers, analytics: analytics, moderationRules: moderationRules,
	}, nil
}

// RegisterHTTP 把已构造的 Handler 注册到 Gin，不再创建业务依赖。
func (a *Application) RegisterHTTP(engine *gin.Engine) {
	router.Register(engine, a.deps.Logger, a.deps.JWT, a.deps.Redis, a.handlers)
}

// StartWorkers 把应用后台任务加入统一生命周期任务组。
func (a *Application) StartWorkers(tasks *bootstrap.TaskGroup) {
	bootstrap.StartNotificationWorker(tasks, a.deps.Config, a.deps.DB, a.deps.Mailer, a.deps.Logger)
	bootstrap.StartModerationReviewEmailWorker(tasks, a.deps.Config, a.deps.DB, a.deps.Mailer, a.deps.Logger)
	bootstrap.StartModerationCleanupWorker(tasks, a.deps.Config, a.deps.DB, a.deps.Store, a.deps.Logger)
	bootstrap.StartAnalyticsWorker(
		tasks, a.deps.Redis, a.deps.Logger, a.analytics.Ingestor, a.analytics.Repo, a.analytics.TZ,
		a.deps.Config.Analytics.RetentionDays, a.deps.Config.Analytics.OnlineWindow,
	)
	if a.moderationRules != nil {
		tasks.Go("moderation-rules", a.moderationRules.Run)
		a.deps.Logger.Info("审核规则构建 worker 启动")
	}
}
