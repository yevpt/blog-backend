package config

import "strings"

// GitHubBridgeMode 控制 GitHub 换码时本机直连与海外 Bridge 的策略。
type GitHubBridgeMode string

const (
	// GitHubBridgeModeDirect 仅本机直连 GitHub。
	GitHubBridgeModeDirect GitHubBridgeMode = "direct"
	// GitHubBridgeModeFallback 先本机直连，换码阶段网络失败时再走 Bridge。
	GitHubBridgeModeFallback GitHubBridgeMode = "fallback"
	// GitHubBridgeModeBridgeOnly 仅走 Bridge；未配置 bridge_url/bridge_secret 时仍回落 direct。
	GitHubBridgeModeBridgeOnly GitHubBridgeMode = "bridge_only"
)

// ParseGitHubBridgeMode 解析 bridge_mode 配置，未知值回落 direct。
func ParseGitHubBridgeMode(raw string) GitHubBridgeMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(GitHubBridgeModeFallback):
		return GitHubBridgeModeFallback
	case string(GitHubBridgeModeBridgeOnly):
		return GitHubBridgeModeBridgeOnly
	default:
		return GitHubBridgeModeDirect
	}
}

// BridgeConfigured 表示 bridge_url 与 bridge_secret 均已配置。
func (c OAuthProviderConfig) BridgeConfigured() bool {
	return strings.TrimSpace(c.BridgeURL) != "" && strings.TrimSpace(c.BridgeSecret) != ""
}

// EffectiveGitHubBridgeMode 返回实际生效的 bridge 策略；未配置 Bridge 时恒为 direct。
func (c OAuthProviderConfig) EffectiveGitHubBridgeMode() GitHubBridgeMode {
	if !c.BridgeConfigured() {
		return GitHubBridgeModeDirect
	}
	return ParseGitHubBridgeMode(c.BridgeMode)
}
