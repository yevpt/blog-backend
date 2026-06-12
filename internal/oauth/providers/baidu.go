package providers

import (
	"context"

	domain "github.com/vpt/blog-backend/internal/oauth"
	"github.com/vpt/blog-backend/pkg/config"
	gooauth2 "golang.org/x/oauth2"
)

const (
	baiduSource   = "baidu"
	baiduAuthURL  = "https://openapi.baidu.com/oauth/2.0/authorize"
	baiduTokenURL = "https://openapi.baidu.com/oauth/2.0/token"
	baiduUserURL  = "https://openapi.baidu.com/rest/2.0/passport/users/getInfo"
)

// BaiduProvider 封装百度 OAuth2 登录差异。
type BaiduProvider struct {
	cfg    config.OAuthProviderConfig
	oauth  *gooauth2.Config
	client httpClient
}

// NewBaiduProvider 创建百度 OAuth provider。
func NewBaiduProvider(cfg config.OAuthProviderConfig) *BaiduProvider {
	applyBaiduDefaults(&cfg)
	return &BaiduProvider{
		cfg: cfg,
		oauth: &gooauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURI,
			Scopes:       cfg.Scopes,
			Endpoint: gooauth2.Endpoint{
				AuthURL:  cfg.AuthURL,
				TokenURL: cfg.TokenURL,
			},
		},
		client: newProviderHTTPClient(),
	}
}

// Source 返回本站内部统一使用的平台标识。
func (p *BaiduProvider) Source() string {
	return baiduSource
}

// AuthCodeURL 生成百度授权地址。
func (p *BaiduProvider) AuthCodeURL(opts domain.AuthCodeOptions) (string, error) {
	return p.oauth.AuthCodeURL(opts.State), nil
}

// Exchange 使用 callback code 换取百度 access token。
func (p *BaiduProvider) Exchange(ctx context.Context, code string, verifier string) (*domain.TokenSet, error) {
	token, err := p.oauth.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	return tokenSetFromOAuth2(token), nil
}

// FetchProfile 拉取百度用户资料。
func (p *BaiduProvider) FetchProfile(ctx context.Context, token *domain.TokenSet) (*domain.Profile, error) {
	endpoint, err := withQuery(p.cfg.UserURL, map[string]string{"access_token": token.AccessToken})
	if err != nil {
		return nil, err
	}
	var user baiduUser
	if err := getJSON(ctx, p.client, "Baidu", endpoint, &user); err != nil {
		return nil, err
	}

	return &domain.Profile{
		Source:    baiduSource,
		UUID:      firstNonEmpty(user.UserID, user.OpenID),
		OpenID:    strPtr(user.OpenID),
		Nickname:  strPtr(user.Username),
		AvatarURL: baiduPortraitURL(user.Portrait),
	}, nil
}

func applyBaiduDefaults(cfg *config.OAuthProviderConfig) {
	if cfg.AuthURL == "" {
		cfg.AuthURL = baiduAuthURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = baiduTokenURL
	}
	if cfg.UserURL == "" {
		cfg.UserURL = baiduUserURL
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"basic"}
	}
}

func baiduPortraitURL(portrait string) *string {
	if portrait == "" {
		return nil
	}
	return strPtr("http://tb.himg.baidu.com/sys/portrait/item/" + portrait)
}

type baiduUser struct {
	UserID   string `json:"userid"`
	OpenID   string `json:"openid"`
	Username string `json:"username"`
	Portrait string `json:"portrait"`
}
