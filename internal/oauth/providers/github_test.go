package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/oauth"
	"github.com/vpt/blog-backend/internal/oauth/providers"
	"github.com/vpt/blog-backend/pkg/config"
)

func TestGitHubProvider_AuthCodeURLIncludesStateScopeAndPKCE(t *testing.T) {
	provider := providers.NewGitHubProvider(config.OAuthProviderConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURI:  "https://api.example.com/oauth/github/callback",
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		Scopes:       []string{"read:user", "user:email"},
	})

	authURL, err := provider.AuthCodeURL(oauth.AuthCodeOptions{
		State:    "state-value",
		Verifier: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz",
	})
	require.NoError(t, err)

	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	query := parsed.Query()
	assert.Equal(t, "state-value", query.Get("state"))
	assert.Equal(t, "client-id", query.Get("client_id"))
	assert.Equal(t, "https://api.example.com/oauth/github/callback", query.Get("redirect_uri"))
	assert.Equal(t, "read:user user:email", query.Get("scope"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
	assert.NotEmpty(t, query.Get("code_challenge"))
}

func TestGitHubProvider_FetchProfileUsesUserEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "/user", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": 12345,
			"login": "octocat",
			"name": "Octo Cat",
			"email": "octo@example.com",
			"avatar_url": "https://avatars.example.com/octo.png",
			"blog": "https://octo.example.com"
		}`))
	}))
	t.Cleanup(server.Close)

	provider := providers.NewGitHubProvider(config.OAuthProviderConfig{UserURL: server.URL + "/user"})

	profile, err := provider.FetchProfile(context.Background(), &oauth.TokenSet{AccessToken: "access-token"})
	require.NoError(t, err)

	assert.Equal(t, "github", profile.Source)
	assert.Equal(t, "12345", profile.UUID)
	assert.Equal(t, "octo@example.com", *profile.Email)
	assert.Equal(t, "Octo Cat", *profile.Nickname)
	assert.Equal(t, "https://avatars.example.com/octo.png", *profile.AvatarURL)
	assert.Equal(t, "https://octo.example.com", *profile.BlogURL)
}

func TestGitHubProvider_FetchProfileLoadsPrimaryVerifiedEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 12345,
				"login": "octocat",
				"name": "",
				"email": "",
				"avatar_url": "https://avatars.example.com/octo.png"
			}`))
		case "/user/emails":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{"email": "secondary@example.com", "primary": false, "verified": true},
				{"email": "primary@example.com", "primary": true, "verified": true}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	provider := providers.NewGitHubProvider(config.OAuthProviderConfig{UserURL: server.URL + "/user"})

	profile, err := provider.FetchProfile(context.Background(), &oauth.TokenSet{AccessToken: "access-token"})
	require.NoError(t, err)

	assert.Equal(t, "octocat", *profile.Nickname)
	assert.Equal(t, "primary@example.com", *profile.Email)
}

func TestGitHubProvider_FetchProfileReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	provider := providers.NewGitHubProvider(config.OAuthProviderConfig{UserURL: server.URL + "/user"})

	_, err := provider.FetchProfile(context.Background(), &oauth.TokenSet{AccessToken: "bad-token"})

	assert.Error(t, err)
}

func TestGitHubProvider_ExchangeProfileUsesBridgeWhenBridgeOnlyConfigured(t *testing.T) {
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/internal/oauth/github/exchange", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"provider":"github",
			"providerUserId":"99",
			"login":"bridge-user",
			"name":"Bridge User",
			"email":"bridge@example.com",
			"emailVerified":true
		}`))
	}))
	t.Cleanup(bridgeServer.Close)

	provider := providers.NewGitHubProvider(config.OAuthProviderConfig{
		RedirectURI:  "https://api.example.com/oauth/github/callback",
		BridgeURL:    bridgeServer.URL,
		BridgeSecret: "bridge-secret",
		BridgeMode:   "bridge_only",
	})

	token, profile, err := provider.ExchangeProfile(context.Background(), "auth-code", "pkce-verifier")
	require.NoError(t, err)
	assert.Empty(t, token.AccessToken)
	assert.Equal(t, "99", profile.UUID)
	assert.Equal(t, "Bridge User", *profile.Nickname)
}

func TestGitHubProvider_ExchangeProfileBridgeOnlyWithoutBridgeFallsBackToDirect(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"direct-token","token_type":"bearer"}`))
	}))
	t.Cleanup(tokenServer.Close)

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":11,"login":"direct-user","name":"Direct User"}`))
	}))
	t.Cleanup(userServer.Close)

	provider := providers.NewGitHubProvider(config.OAuthProviderConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURI:  "https://api.example.com/oauth/github/callback",
		TokenURL:     tokenServer.URL,
		UserURL:      userServer.URL + "/user",
		BridgeMode:   "bridge_only",
	})

	token, profile, err := provider.ExchangeProfile(context.Background(), "auth-code", "")
	require.NoError(t, err)
	assert.Equal(t, "direct-token", token.AccessToken)
	assert.Equal(t, "11", profile.UUID)
}

func TestGitHubProvider_ExchangeProfileFallbackUsesBridgeAfterDirectExchangeNetworkFailure(t *testing.T) {
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"provider":"github",
			"providerUserId":"77",
			"login":"fallback-user",
			"name":"Fallback User",
			"emailVerified":false
		}`))
	}))
	t.Cleanup(bridgeServer.Close)

	provider := providers.NewGitHubProvider(config.OAuthProviderConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURI:  "https://api.example.com/oauth/github/callback",
		TokenURL:     "http://127.0.0.1:1/oauth/token",
		BridgeURL:    bridgeServer.URL,
		BridgeSecret: "bridge-secret",
		BridgeMode:   "fallback",
	})

	token, profile, err := provider.ExchangeProfile(context.Background(), "auth-code", "")
	require.NoError(t, err)
	assert.Empty(t, token.AccessToken)
	assert.Equal(t, "77", profile.UUID)
	assert.Equal(t, "Fallback User", *profile.Nickname)
}

func TestGitHubProvider_ExchangeProfileUsesDirectGitHubWhenBridgeNotConfigured(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"direct-token","token_type":"bearer"}`))
	}))
	t.Cleanup(tokenServer.Close)

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer direct-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"id":11,"login":"direct-user","name":"Direct User"}`))
	}))
	t.Cleanup(userServer.Close)

	provider := providers.NewGitHubProvider(config.OAuthProviderConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURI:  "https://api.example.com/oauth/github/callback",
		TokenURL:     tokenServer.URL,
		UserURL:      userServer.URL + "/user",
	})

	token, profile, err := provider.ExchangeProfile(context.Background(), "auth-code", "")
	require.NoError(t, err)
	assert.Equal(t, "direct-token", token.AccessToken)
	assert.Equal(t, "11", profile.UUID)
	assert.Equal(t, "Direct User", *profile.Nickname)
}
