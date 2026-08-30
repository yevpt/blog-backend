package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/pkg/config"
)

func TestNewOAuthManagerRegistersEnabledSocialProviders(t *testing.T) {
	cfg := &config.Config{
		OAuth: config.OAuthConfig{
			Providers: map[string]config.OAuthProviderConfig{
				"github": {Enabled: true},
				"gitee":  {Enabled: true},
				"qq":     {Enabled: true},
				"weibo":  {Enabled: true},
				"baidu":  {Enabled: true},
				"google": {Enabled: false},
			},
		},
	}

	manager := newOAuthManager(nil, cfg)

	assert.Equal(t, []string{"baidu", "gitee", "github", "qq", "weibo"}, manager.Sources())
}
