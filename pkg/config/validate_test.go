package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/config"
)

func TestConfigValidateAcceptsProductionConfig(t *testing.T) {
	cfg := validProductionConfig()
	require.NoError(t, cfg.Validate("prod"))
}

func TestConfigValidateRejectsInvalidRuntimeBounds(t *testing.T) {
	cfg := validProductionConfig()
	cfg.DB.MaxIdleConns = cfg.DB.MaxOpenConns + 1

	require.ErrorContains(t, cfg.Validate("test"), "db: 连接池参数无效")
}

func TestConfigValidateRejectsUnsafeProductionConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string
	}{
		{name: "debug mode", mutate: func(c *config.Config) { c.Server.Mode = "debug" }, wantErr: "server.mode"},
		{name: "placeholder jwt", mutate: func(c *config.Config) { c.JWT.Secret = "change-me-in-production" }, wantErr: "jwt.secret"},
		{name: "missing database", mutate: func(c *config.Config) { c.DB.Password = "" }, wantErr: "db:"},
		{name: "missing redis", mutate: func(c *config.Config) { c.Redis.Addr = "" }, wantErr: "redis.addr"},
		{name: "missing storage", mutate: func(c *config.Config) { c.Garage.Bucket = "" }, wantErr: "garage:"},
		{name: "missing email", mutate: func(c *config.Config) { c.Email.Password = "" }, wantErr: "email:"},
		{name: "placeholder analytics salt", mutate: func(c *config.Config) { c.Analytics.IPSalt = "change_me" }, wantErr: "analytics.ip_salt"},
		{name: "missing site host", mutate: func(c *config.Config) { c.Analytics.SiteHost = "" }, wantErr: "analytics.site_host"},
		{name: "short collect token", mutate: func(c *config.Config) { c.Analytics.CollectTokenSecret = "short" }, wantErr: "analytics.collect_token_secret"},
		{name: "missing cdn origin secret", mutate: func(c *config.Config) {
			c.Garage.CDN = true
			c.CDN = config.CDNConfig{Host: "https://cdn.example.com", Secret: "cdn-secret", SignQueryName: "a", TimestampQueryName: "b"}
		}, wantErr: "image.originAuthSecret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validProductionConfig()
			tt.mutate(&cfg)
			require.ErrorContains(t, cfg.Validate("prod"), tt.wantErr)
		})
	}
}

func TestConfigValidateRejectsIncompleteEnabledOAuthProvider(t *testing.T) {
	cfg := validProductionConfig()
	cfg.OAuth.Providers = map[string]config.OAuthProviderConfig{
		"github": {Enabled: true},
	}

	require.ErrorContains(t, cfg.Validate("test"), "oauth.providers.github")
}

func validProductionConfig() config.Config {
	return config.Config{
		Server: config.ServerConfig{Port: 8080, Mode: "release"},
		JWT: config.JWTConfig{
			Secret: strings.Repeat("j", 32), ExpireHours: 2, RefreshExpireHours: 168,
		},
		DB: config.DBConfig{
			Host: "127.0.0.1", Port: 3306, Name: "blog", User: "blog", Password: "db-secret",
			MaxOpenConns: 10, MaxIdleConns: 5, MaxLifetimeMinutes: 30,
		},
		Redis: config.RedisConfig{Addr: "127.0.0.1:6379"},
		Garage: config.GarageConfig{
			Endpoint: "http://garage.example.com", Bucket: "blog", Region: "garage",
			AccessKeyID: "garage-access", SecretAccessKey: "garage-secret",
		},
		Email: config.EmailConfig{
			Host: "smtp.example.com", Port: 465, From: "noreply@example.com", Password: "email-secret",
			ProviderDailyHardLimit: 2000, SiteDailySafeLimit: 300, MaxPerMinute: 5, MaxPerHour: 80,
			SendIntervalSeconds: 12, WorkerEnabled: true, PlannerEnabled: true, WorkerBatchSize: 20, LeaseSeconds: 300,
		},
		OAuth: config.OAuthConfig{StateTTLMinutes: 10},
		Analytics: config.AnalyticsConfig{
			Timezone: "Asia/Shanghai", RetentionDays: 90, OnlineWindow: 90 * time.Second,
			SessionTimeout: 30 * time.Minute, BounceDuration: 10 * time.Second, ChannelBuffer: 4096,
			PublicCacheTTL: time.Minute, IPSalt: strings.Repeat("s", 32), SiteHost: "example.com", CollectTokenTTL: 6 * time.Minute,
		},
		Image: config.ImageConfig{ResponseCacheMaxAge: 604800, DefaultQuality: 75, MaxWidth: 3840},
	}
}
