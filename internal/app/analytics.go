package app

import (
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	analyticshandler "github.com/vpt/blog-backend/internal/handler/analytics"
	analyticsrepo "github.com/vpt/blog-backend/internal/repository/analytics"
	analyticsservice "github.com/vpt/blog-backend/internal/service/analytics"
	"github.com/vpt/blog-backend/internal/service/uv"
	analyticsworker "github.com/vpt/blog-backend/internal/worker/analytics"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const analyticsAllowedOriginsEnv = "ANALYTICS_ALLOWED_ORIGINS"

func newAnalyticsHandlers(
	logger *zap.Logger,
	db *gorm.DB,
	redisClient *redis.Client,
	uvSvc uv.UVService,
	analyticsCfg config.AnalyticsConfig,
	userPresence analyticsservice.UserPresence,
) (*analyticshandler.CollectHandler, *analyticshandler.AdminHandler, *analyticshandler.PublicHandler, analyticsRuntime) {
	tz, err := time.LoadLocation(analyticsCfg.Timezone)
	if err != nil {
		logger.Warn("统计时区解析失败，回退东八区", zap.String("timezone", analyticsCfg.Timezone), zap.Error(err))
		tz = time.FixedZone("CST", 8*3600)
	}

	geo := analyticsservice.NewGeoResolver(analyticsCfg.GeoIPV4Path, analyticsCfg.GeoIPV6Path, logger)
	enricher := analyticsservice.NewEnricher(geo, analyticsCfg.SiteHost, analyticsCfg.IPSalt)
	repo := analyticsrepo.NewRepository(db)
	realtime := analyticsservice.NewRealtime(redisClient, tz, analyticsCfg.OnlineWindow)
	ingestor := analyticsworker.NewIngestor(repo, analyticsCfg.ChannelBuffer, 100, 2*time.Second, logger)
	sessionIngestor := analyticsworker.NewSessionIngestor(ingestor, repo)
	dedup := analyticsservice.NewDedupChecker(uvSvc)
	tokenVerifier := analyticsservice.NewCollectTokenVerifier(analyticsCfg.CollectTokenSecret, analyticsCfg.CollectTokenTTL, nil)
	collectSvc := analyticsservice.NewCollectService(enricher, realtime, sessionIngestor, dedup, tokenVerifier, userPresence, logger)

	querySvc := analyticsservice.NewQueryService(repo, realtime, logger)
	backfillRollup := analyticsworker.NewRollup(repo, repo, logger)
	adminHandler := analyticshandler.NewAdminHandler(
		querySvc, analyticsservice.NewBackfillService(backfillRollup.RollupDay),
	)
	publicHandler := analyticshandler.NewPublicHandler(
		analyticsservice.NewPublicService(repo, realtime, redisClient, analyticsCfg.PublicCacheTTL, logger),
	)

	allowedOrigins := splitOrigins(os.Getenv(analyticsAllowedOriginsEnv))
	runtime := analyticsRuntime{Ingestor: ingestor, Repo: repo, TZ: tz}
	return analyticshandler.NewCollectHandler(collectSvc, allowedOrigins), adminHandler, publicHandler, runtime
}

func splitOrigins(value string) []string {
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
