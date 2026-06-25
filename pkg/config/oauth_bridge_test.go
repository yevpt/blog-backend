package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vpt/blog-backend/pkg/config"
)

func TestParseGitHubBridgeMode(t *testing.T) {
	assert.Equal(t, config.GitHubBridgeModeDirect, config.ParseGitHubBridgeMode(""))
	assert.Equal(t, config.GitHubBridgeModeDirect, config.ParseGitHubBridgeMode("direct"))
	assert.Equal(t, config.GitHubBridgeModeFallback, config.ParseGitHubBridgeMode("fallback"))
	assert.Equal(t, config.GitHubBridgeModeBridgeOnly, config.ParseGitHubBridgeMode("bridge_only"))
	assert.Equal(t, config.GitHubBridgeModeDirect, config.ParseGitHubBridgeMode("unknown"))
}

func TestOAuthProviderConfig_EffectiveGitHubBridgeMode(t *testing.T) {
	cfg := config.OAuthProviderConfig{BridgeMode: "bridge_only"}
	assert.Equal(t, config.GitHubBridgeModeDirect, cfg.EffectiveGitHubBridgeMode())

	cfg = config.OAuthProviderConfig{
		BridgeMode:   "bridge_only",
		BridgeURL:    "https://bridge.example.com",
		BridgeSecret: "secret",
	}
	assert.Equal(t, config.GitHubBridgeModeBridgeOnly, cfg.EffectiveGitHubBridgeMode())

	cfg = config.OAuthProviderConfig{
		BridgeMode: "fallback",
		BridgeURL:  "https://bridge.example.com",
	}
	assert.Equal(t, config.GitHubBridgeModeDirect, cfg.EffectiveGitHubBridgeMode())
}
