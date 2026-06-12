package providers

import (
	"context"

	domain "github.com/vpt/blog-backend/internal/oauth"
	"github.com/vpt/blog-backend/pkg/config"
	gooauth2 "golang.org/x/oauth2"
)

const (
	giteeSource   = "gitee"
	giteeAuthURL  = "https://gitee.com/oauth/authorize"
	giteeTokenURL = "https://gitee.com/oauth/token"
	giteeUserURL  = "https://gitee.com/api/v5/user"
)

// GiteeProvider 封装 Gitee OAuth2 登录差异。
type GiteeProvider struct {
	cfg    config.OAuthProviderConfig
	oauth  *gooauth2.Config
	client httpClient
}

// NewGiteeProvider 创建 Gitee OAuth provider。
func NewGiteeProvider(cfg config.OAuthProviderConfig) *GiteeProvider {
	applyGiteeDefaults(&cfg)
	return &GiteeProvider{
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
func (p *GiteeProvider) Source() string {
	return giteeSource
}

// AuthCodeURL 生成 Gitee 授权地址。
func (p *GiteeProvider) AuthCodeURL(opts domain.AuthCodeOptions) (string, error) {
	return p.oauth.AuthCodeURL(opts.State), nil
}

// Exchange 使用 callback code 换取 Gitee access token。
func (p *GiteeProvider) Exchange(ctx context.Context, code string, verifier string) (*domain.TokenSet, error) {
	token, err := p.oauth.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	return tokenSetFromOAuth2(token), nil
}

// FetchProfile 拉取 Gitee 用户资料。
func (p *GiteeProvider) FetchProfile(ctx context.Context, token *domain.TokenSet) (*domain.Profile, error) {
	var user giteeUser
	if err := getJSONWithBearer(ctx, p.client, "Gitee", p.cfg.UserURL, token.AccessToken, &user); err != nil {
		return nil, err
	}

	nickname := strPtr(user.Name)
	if nickname == nil {
		nickname = strPtr(user.Login)
	}
	return &domain.Profile{
		Source:    giteeSource,
		UUID:      user.ID.String(),
		Email:     strPtr(user.Email),
		Nickname:  nickname,
		AvatarURL: strPtr(user.AvatarURL),
		BlogURL:   strPtr(user.Blog),
	}, nil
}

func applyGiteeDefaults(cfg *config.OAuthProviderConfig) {
	if cfg.AuthURL == "" {
		cfg.AuthURL = giteeAuthURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = giteeTokenURL
	}
	if cfg.UserURL == "" {
		cfg.UserURL = giteeUserURL
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"user_info", "emails"}
	}
}

type giteeUser struct {
	ID        jsonNumber `json:"id"`
	Login     string     `json:"login"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	AvatarURL string     `json:"avatar_url"`
	Blog      string     `json:"blog"`
}
