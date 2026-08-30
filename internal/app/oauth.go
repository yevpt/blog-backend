package app

import (
	"time"

	"github.com/redis/go-redis/v9"
	oauthflow "github.com/vpt/blog-backend/internal/oauth"
	oauthproviders "github.com/vpt/blog-backend/internal/oauth/providers"
	"github.com/vpt/blog-backend/pkg/config"
)

func newOAuthManager(redisClient *redis.Client, cfg *config.Config) *oauthflow.Manager {
	var ttl time.Duration
	if cfg != nil && cfg.OAuth.StateTTLMinutes > 0 {
		ttl = time.Duration(cfg.OAuth.StateTTLMinutes) * time.Minute
	}

	providers := make([]oauthflow.Provider, 0)
	if cfg != nil {
		if provider, ok := cfg.OAuth.Providers["github"]; ok && provider.Enabled {
			providers = append(providers, oauthproviders.NewGitHubProvider(provider))
		}
		if provider, ok := cfg.OAuth.Providers["gitee"]; ok && provider.Enabled {
			providers = append(providers, oauthproviders.NewGiteeProvider(provider))
		}
		if provider, ok := cfg.OAuth.Providers["qq"]; ok && provider.Enabled {
			providers = append(providers, oauthproviders.NewQQProvider(provider))
		}
		if provider, ok := cfg.OAuth.Providers["weibo"]; ok && provider.Enabled {
			providers = append(providers, oauthproviders.NewWeiboProvider(provider))
		}
		if provider, ok := cfg.OAuth.Providers["baidu"]; ok && provider.Enabled {
			providers = append(providers, oauthproviders.NewBaiduProvider(provider))
		}
	}
	return oauthflow.NewManager(oauthflow.NewStateStore(redisClient, ttl), providers)
}
