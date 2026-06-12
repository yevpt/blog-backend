package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	domain "github.com/vpt/blog-backend/internal/oauth"
	"github.com/vpt/blog-backend/pkg/config"
	gooauth2 "golang.org/x/oauth2"
)

const (
	qqSource    = "qq"
	qqAuthURL   = "https://graph.qq.com/oauth2.0/authorize"
	qqTokenURL  = "https://graph.qq.com/oauth2.0/token"
	qqOpenIDURL = "https://graph.qq.com/oauth2.0/me"
	qqUserURL   = "https://graph.qq.com/user/get_user_info"
)

// QQProvider 封装 QQ 互联 OAuth2 登录差异。
type QQProvider struct {
	cfg    config.OAuthProviderConfig
	oauth  *gooauth2.Config
	client httpClient
}

// NewQQProvider 创建 QQ OAuth provider。
func NewQQProvider(cfg config.OAuthProviderConfig) *QQProvider {
	applyQQDefaults(&cfg)
	return &QQProvider{
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
func (p *QQProvider) Source() string {
	return qqSource
}

// AuthCodeURL 生成 QQ 授权地址。
func (p *QQProvider) AuthCodeURL(opts domain.AuthCodeOptions) (string, error) {
	return p.oauth.AuthCodeURL(opts.State), nil
}

// Exchange 使用 callback code 换取 QQ access token。
func (p *QQProvider) Exchange(ctx context.Context, code string, verifier string) (*domain.TokenSet, error) {
	endpoint, err := withQuery(p.cfg.TokenURL, map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     p.cfg.ClientID,
		"client_secret": p.cfg.ClientSecret,
		"code":          code,
		"redirect_uri":  p.cfg.RedirectURI,
	})
	if err != nil {
		return nil, err
	}
	body, err := getText(ctx, p.client, "QQ", endpoint)
	if err != nil {
		return nil, err
	}
	return qqTokenSetFromForm(body)
}

// FetchProfile 先拉取 openid，再用 openid 拉取 QQ 用户资料。
func (p *QQProvider) FetchProfile(ctx context.Context, token *domain.TokenSet) (*domain.Profile, error) {
	openID, err := p.fetchOpenID(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	user, err := p.fetchUserInfo(ctx, token.AccessToken, openID)
	if err != nil {
		return nil, err
	}

	return &domain.Profile{
		Source:    qqSource,
		UUID:      openID,
		OpenID:    strPtr(openID),
		Nickname:  strPtr(user.Nickname),
		AvatarURL: strPtr(firstNonEmpty(user.FigureURLQQ2, user.FigureURLQQ1, user.FigureURL2, user.FigureURL1)),
	}, nil
}

func (p *QQProvider) fetchOpenID(ctx context.Context, accessToken string) (string, error) {
	endpoint, err := withQuery(p.cfg.OpenIDURL, map[string]string{"access_token": accessToken})
	if err != nil {
		return "", err
	}
	body, err := getText(ctx, p.client, "QQ", endpoint)
	if err != nil {
		return "", err
	}
	var resp qqOpenIDResp
	if err := json.Unmarshal([]byte(stripJSONP(body)), &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.OpenID) == "" {
		return "", fmt.Errorf("QQ openid 响应缺少 openid")
	}
	return resp.OpenID, nil
}

func (p *QQProvider) fetchUserInfo(ctx context.Context, accessToken string, openID string) (*qqUser, error) {
	endpoint, err := withQuery(p.cfg.UserURL, map[string]string{
		"access_token":       accessToken,
		"oauth_consumer_key": p.cfg.ClientID,
		"openid":             openID,
	})
	if err != nil {
		return nil, err
	}
	var user qqUser
	if err := getJSON(ctx, p.client, "QQ", endpoint, &user); err != nil {
		return nil, err
	}
	if user.Ret != 0 {
		return nil, fmt.Errorf("QQ userinfo 请求失败: ret=%d msg=%s", user.Ret, user.Msg)
	}
	return &user, nil
}

func applyQQDefaults(cfg *config.OAuthProviderConfig) {
	if cfg.AuthURL == "" {
		cfg.AuthURL = qqAuthURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = qqTokenURL
	}
	if cfg.OpenIDURL == "" {
		cfg.OpenIDURL = qqOpenIDURL
	}
	if cfg.UserURL == "" {
		cfg.UserURL = qqUserURL
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"get_user_info"}
	}
}

func stripJSONP(body string) string {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "callback(") {
		trimmed = strings.TrimPrefix(trimmed, "callback(")
		trimmed = strings.TrimSuffix(trimmed, ");")
	}
	return strings.TrimSpace(trimmed)
}

type qqOpenIDResp struct {
	ClientID string `json:"client_id"`
	OpenID   string `json:"openid"`
}

func qqTokenSetFromForm(body string) (*domain.TokenSet, error) {
	values, err := url.ParseQuery(strings.TrimSpace(body))
	if err != nil {
		return nil, err
	}
	if values.Get("error") != "" {
		return nil, fmt.Errorf("QQ token 请求失败: error=%s description=%s", values.Get("error"), values.Get("error_description"))
	}
	accessToken := values.Get("access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("QQ token 响应缺少 access_token")
	}

	var refreshToken *string
	if raw := values.Get("refresh_token"); raw != "" {
		refreshToken = &raw
	}
	var expiry *time.Time
	if raw := values.Get("expires_in"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			value := time.Now().Add(time.Duration(seconds) * time.Second)
			expiry = &value
		}
	}
	return &domain.TokenSet{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       expiry,
	}, nil
}

type qqUser struct {
	Ret          int    `json:"ret"`
	Msg          string `json:"msg"`
	Nickname     string `json:"nickname"`
	FigureURL1   string `json:"figureurl_1"`
	FigureURL2   string `json:"figureurl_2"`
	FigureURLQQ1 string `json:"figureurl_qq_1"`
	FigureURLQQ2 string `json:"figureurl_qq_2"`
}
