package oauth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidAction 表示授权动作不是当前系统支持的 login 或 bind。
	ErrInvalidAction = errors.New("无效的 OAuth 授权动作")
	// ErrInvalidState 表示 OAuth callback 携带的 state 不存在、已过期或已被消费。
	ErrInvalidState = errors.New("OAuth state 无效或已过期")
	// ErrProviderNotEnabled 表示请求的平台没有配置或未启用。
	ErrProviderNotEnabled = errors.New("OAuth 平台未启用")
	// ErrStateSourceMismatch 表示 callback 路径平台与 state 中记录的平台不一致。
	ErrStateSourceMismatch = errors.New("OAuth state 平台不匹配")
)

// Action 表示一次 OAuth 流程的业务目的。
type Action string

const (
	// ActionLogin 表示第三方账号登录本站。
	ActionLogin Action = "login"
	// ActionBind 表示把第三方账号绑定到当前登录用户。
	ActionBind Action = "bind"
)

// ParseAction 将请求中的 action 参数转换为受控枚举。
func ParseAction(raw string) (Action, error) {
	switch Action(raw) {
	case ActionLogin:
		return ActionLogin, nil
	case ActionBind:
		return ActionBind, nil
	default:
		return "", ErrInvalidAction
	}
}

// Profile 是第三方平台用户资料在本站内部的统一表达。
type Profile struct {
	Source    string  // 平台标识，例如 github、gitee、google
	UUID      string  // 第三方平台稳定用户 ID，用于认证身份匹配
	OpenID    *string // 部分平台返回的 openid，保留用于后续扩展
	Email     *string // 第三方平台返回的邮箱，可能为空或未验证
	Nickname  *string // 第三方昵称
	AvatarURL *string // 第三方头像 URL
	BlogURL   *string // 第三方个人主页或站点
}

// IdentityKey 返回便于日志和测试断言使用的身份标识。
func (p Profile) IdentityKey() string {
	return fmt.Sprintf("%s:%s", p.Source, p.UUID)
}

// TokenSet 是第三方平台 token 的统一表达，不会直接返回给前端。
type TokenSet struct {
	AccessToken  string
	RefreshToken *string
	IDToken      *string
	Expiry       *time.Time
}

// AuthCodeOptions 是生成授权 URL 时传给 provider 的安全上下文。
type AuthCodeOptions struct {
	State    string
	Verifier string
}

// Provider 封装单个第三方平台的 OAuth 差异。
type Provider interface {
	// Source 返回平台标识，必须与配置和数据库 social_user.source 保持一致。
	Source() string
	// AuthCodeURL 生成第三方授权地址，必须携带 state，并在支持时携带 PKCE challenge。
	AuthCodeURL(opts AuthCodeOptions) (string, error)
	// Exchange 使用 callback code 换取第三方 token。
	Exchange(ctx context.Context, code string, verifier string) (*TokenSet, error)
	// FetchProfile 使用第三方 token 拉取用户资料，并转换为统一 Profile。
	FetchProfile(ctx context.Context, token *TokenSet) (*Profile, error)
}
