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

func TestGiteeProvider_FetchProfileMapsUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "/api/v5/user", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": 12345,
			"login": "gitee-user",
			"name": "Gitee User",
			"email": "gitee@example.com",
			"avatar_url": "https://gitee.example.com/avatar.png",
			"blog": "https://gitee.example.com/u/gitee-user"
		}`))
	}))
	t.Cleanup(server.Close)

	provider := providers.NewGiteeProvider(config.OAuthProviderConfig{UserURL: server.URL + "/api/v5/user"})

	profile, err := provider.FetchProfile(context.Background(), &oauth.TokenSet{AccessToken: "access-token"})
	require.NoError(t, err)

	assert.Equal(t, "gitee", profile.Source)
	assert.Equal(t, "12345", profile.UUID)
	assert.Equal(t, "gitee@example.com", *profile.Email)
	assert.Equal(t, "Gitee User", *profile.Nickname)
	assert.Equal(t, "https://gitee.example.com/avatar.png", *profile.AvatarURL)
	assert.Equal(t, "https://gitee.example.com/u/gitee-user", *profile.BlogURL)
}

func TestBaiduProvider_FetchProfileMapsUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "access-token", r.URL.Query().Get("access_token"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"userid": "baidu-user-id",
			"username": "baidu_user",
			"portrait": "avatar-token"
		}`))
	}))
	t.Cleanup(server.Close)

	provider := providers.NewBaiduProvider(config.OAuthProviderConfig{UserURL: server.URL + "/rest/2.0/passport/users/getInfo"})

	profile, err := provider.FetchProfile(context.Background(), &oauth.TokenSet{AccessToken: "access-token"})
	require.NoError(t, err)

	assert.Equal(t, "baidu", profile.Source)
	assert.Equal(t, "baidu-user-id", profile.UUID)
	assert.Equal(t, "baidu_user", *profile.Nickname)
	assert.Equal(t, "http://tb.himg.baidu.com/sys/portrait/item/avatar-token", *profile.AvatarURL)
}

func TestWeiboProvider_ExchangePreservesUIDAndFetchProfileMapsUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/access_token":
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
			assert.Equal(t, "callback-code", r.Form.Get("code"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","uid":"weibo-uid","expires_in":3600}`))
		case "/2/users/show.json":
			assert.Equal(t, "access-token", r.URL.Query().Get("access_token"))
			assert.Equal(t, "weibo-uid", r.URL.Query().Get("uid"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"idstr": "weibo-uid",
				"screen_name": "Weibo User",
				"profile_image_url": "https://weibo.example.com/avatar.png",
				"url": "https://weibo.example.com/u/weibo-uid"
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	provider := providers.NewWeiboProvider(config.OAuthProviderConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURI:  "https://api.example.com/oauth/weibo/callback",
		TokenURL:     server.URL + "/oauth2/access_token",
		UserURL:      server.URL + "/2/users/show.json",
	})

	token, err := provider.Exchange(context.Background(), "callback-code", "")
	require.NoError(t, err)
	assert.Equal(t, "weibo-uid", token.Extra["uid"])

	profile, err := provider.FetchProfile(context.Background(), token)
	require.NoError(t, err)

	assert.Equal(t, "weibo", profile.Source)
	assert.Equal(t, "weibo-uid", profile.UUID)
	assert.Equal(t, "Weibo User", *profile.Nickname)
	assert.Equal(t, "https://weibo.example.com/avatar.png", *profile.AvatarURL)
	assert.Equal(t, "https://weibo.example.com/u/weibo-uid", *profile.BlogURL)
}

func TestQQProvider_FetchProfileLoadsOpenIDThenUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2.0/me":
			assert.Equal(t, "access-token", r.URL.Query().Get("access_token"))
			_, _ = w.Write([]byte(`callback( {"client_id":"qq-client-id","openid":"qq-openid"} );`))
		case "/user/get_user_info":
			query := r.URL.Query()
			assert.Equal(t, "access-token", query.Get("access_token"))
			assert.Equal(t, "qq-client-id", query.Get("oauth_consumer_key"))
			assert.Equal(t, "qq-openid", query.Get("openid"))
			_, _ = w.Write([]byte(`{
				"ret": 0,
				"nickname": "QQ User",
				"figureurl_qq_2": "https://qq.example.com/avatar-100.png",
				"figureurl_qq_1": "https://qq.example.com/avatar-40.png"
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	provider := providers.NewQQProvider(config.OAuthProviderConfig{
		ClientID:  "qq-client-id",
		OpenIDURL: server.URL + "/oauth2.0/me",
		UserURL:   server.URL + "/user/get_user_info",
	})

	profile, err := provider.FetchProfile(context.Background(), &oauth.TokenSet{AccessToken: "access-token"})
	require.NoError(t, err)

	assert.Equal(t, "qq", profile.Source)
	assert.Equal(t, "qq-openid", profile.UUID)
	assert.Equal(t, "qq-openid", *profile.OpenID)
	assert.Equal(t, "QQ User", *profile.Nickname)
	assert.Equal(t, "https://qq.example.com/avatar-100.png", *profile.AvatarURL)
}

func TestQQProvider_ExchangeUsesGETAndParsesFormToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		query := r.URL.Query()
		assert.Equal(t, "authorization_code", query.Get("grant_type"))
		assert.Equal(t, "qq-client-id", query.Get("client_id"))
		assert.Equal(t, "qq-client-secret", query.Get("client_secret"))
		assert.Equal(t, "callback-code", query.Get("code"))
		assert.Equal(t, "https://api.example.com/oauth/qq/callback", query.Get("redirect_uri"))
		_, _ = w.Write([]byte(`access_token=qq-access-token&expires_in=5184000&refresh_token=qq-refresh-token`))
	}))
	t.Cleanup(server.Close)

	provider := providers.NewQQProvider(config.OAuthProviderConfig{
		ClientID:     "qq-client-id",
		ClientSecret: "qq-client-secret",
		RedirectURI:  "https://api.example.com/oauth/qq/callback",
		TokenURL:     server.URL + "/oauth2.0/token",
	})

	token, err := provider.Exchange(context.Background(), "callback-code", "")
	require.NoError(t, err)

	assert.Equal(t, "qq-access-token", token.AccessToken)
	require.NotNil(t, token.RefreshToken)
	assert.Equal(t, "qq-refresh-token", *token.RefreshToken)
	require.NotNil(t, token.Expiry)
}

func TestSocialProviders_AuthCodeURLIncludesState(t *testing.T) {
	tests := []struct {
		name     string
		provider oauth.Provider
	}{
		{
			name: "gitee",
			provider: providers.NewGiteeProvider(config.OAuthProviderConfig{
				ClientID:    "client-id",
				RedirectURI: "https://api.example.com/oauth/gitee/callback",
				AuthURL:     "https://gitee.com/oauth/authorize",
			}),
		},
		{
			name: "baidu",
			provider: providers.NewBaiduProvider(config.OAuthProviderConfig{
				ClientID:    "client-id",
				RedirectURI: "https://api.example.com/oauth/baidu/callback",
				AuthURL:     "https://openapi.baidu.com/oauth/2.0/authorize",
			}),
		},
		{
			name: "weibo",
			provider: providers.NewWeiboProvider(config.OAuthProviderConfig{
				ClientID:    "client-id",
				RedirectURI: "https://api.example.com/oauth/weibo/callback",
				AuthURL:     "https://api.weibo.com/oauth2/authorize",
			}),
		},
		{
			name: "qq",
			provider: providers.NewQQProvider(config.OAuthProviderConfig{
				ClientID:    "client-id",
				RedirectURI: "https://api.example.com/oauth/qq/callback",
				AuthURL:     "https://graph.qq.com/oauth2.0/authorize",
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authURL, err := tt.provider.AuthCodeURL(oauth.AuthCodeOptions{State: "state-value", Verifier: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"})
			require.NoError(t, err)

			parsed, err := url.Parse(authURL)
			require.NoError(t, err)
			assert.Equal(t, "state-value", parsed.Query().Get("state"))
			assert.Equal(t, "client-id", parsed.Query().Get("client_id"))
		})
	}
}
