package providers

import (
	"context"
	"fmt"

	domain "github.com/vpt/blog-backend/internal/oauth"
	"github.com/vpt/blog-backend/pkg/config"
	gooauth2 "golang.org/x/oauth2"
)

const (
	weiboSource   = "weibo"
	weiboAuthURL  = "https://api.weibo.com/oauth2/authorize"
	weiboTokenURL = "https://api.weibo.com/oauth2/access_token"
	weiboUserURL  = "https://api.weibo.com/2/users/show.json"
)

// WeiboProvider 封装微博 OAuth2 登录差异。
type WeiboProvider struct {
	cfg    config.OAuthProviderConfig
	oauth  *gooauth2.Config
	client httpClient
}

// NewWeiboProvider 创建微博 OAuth provider。
func NewWeiboProvider(cfg config.OAuthProviderConfig) *WeiboProvider {
	applyWeiboDefaults(&cfg)
	return &WeiboProvider{
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
func (p *WeiboProvider) Source() string {
	return weiboSource
}

// AuthCodeURL 生成微博授权地址。
func (p *WeiboProvider) AuthCodeURL(opts domain.AuthCodeOptions) (string, error) {
	return p.oauth.AuthCodeURL(opts.State), nil
}

// Exchange 使用 callback code 换取微博 access token，并保留 uid。
func (p *WeiboProvider) Exchange(ctx context.Context, code string, verifier string) (*domain.TokenSet, error) {
	token, err := p.oauth.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	return tokenSetFromOAuth2(token), nil
}

// FetchProfile 拉取微博用户资料。
func (p *WeiboProvider) FetchProfile(ctx context.Context, token *domain.TokenSet) (*domain.Profile, error) {
	uid := token.Extra["uid"]
	if uid == "" {
		return nil, fmt.Errorf("Weibo token 缺少 uid")
	}
	endpoint, err := withQuery(p.cfg.UserURL, map[string]string{
		"access_token": token.AccessToken,
		"uid":          uid,
	})
	if err != nil {
		return nil, err
	}
	var user weiboUser
	if err := getJSON(ctx, p.client, "Weibo", endpoint, &user); err != nil {
		return nil, err
	}

	return &domain.Profile{
		Source:    weiboSource,
		UUID:      firstNonEmpty(user.IDStr, uid),
		Nickname:  strPtr(user.ScreenName),
		AvatarURL: strPtr(firstNonEmpty(user.AvatarLarge, user.ProfileImageURL)),
		BlogURL:   strPtr(user.URL),
	}, nil
}

func applyWeiboDefaults(cfg *config.OAuthProviderConfig) {
	if cfg.AuthURL == "" {
		cfg.AuthURL = weiboAuthURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = weiboTokenURL
	}
	if cfg.UserURL == "" {
		cfg.UserURL = weiboUserURL
	}
}

type weiboUser struct {
	IDStr           string `json:"idstr"`
	ScreenName      string `json:"screen_name"`
	ProfileImageURL string `json:"profile_image_url"`
	AvatarLarge     string `json:"avatar_large"`
	URL             string `json:"url"`
}
