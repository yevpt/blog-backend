package oauth

import (
	"context"
	"sort"

	gooauth2 "golang.org/x/oauth2"
)

// CallbackResult 是 OAuth callback 完成后的统一结果。
type CallbackResult struct {
	Flow    *FlowContext // state 中保存的业务流程上下文
	Token   *TokenSet    // 第三方 token，仅供后端持久化或调用平台接口
	Profile *Profile     // 第三方用户资料，用于登录或绑定
}

// Manager 负责串联 state、PKCE、provider 换码和用户资料拉取。
type Manager struct {
	store     *StateStore
	providers map[string]Provider
}

// NewManager 创建 OAuth manager，并按 source 建立 provider 索引。
func NewManager(store *StateStore, providers []Provider) *Manager {
	index := make(map[string]Provider, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		index[provider.Source()] = provider
	}
	return &Manager{store: store, providers: index}
}

// Sources 返回当前已注册 provider 的平台标识，按字母序稳定输出。
func (m *Manager) Sources() []string {
	sources := make([]string, 0, len(m.providers))
	for source := range m.providers {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

// Authorize 创建一次授权流程，并返回第三方授权地址。
func (m *Manager) Authorize(ctx context.Context, source string, flow FlowContext) (string, error) {
	provider, ok := m.providers[source]
	if !ok {
		return "", ErrProviderNotEnabled
	}

	// verifier 与 state 一起保存在 Redis，callback 换 token 时用于 PKCE 校验。
	flow.Source = source
	flow.Verifier = gooauth2.GenerateVerifier()
	state, err := m.store.Create(ctx, flow)
	if err != nil {
		return "", err
	}

	return provider.AuthCodeURL(AuthCodeOptions{
		State:    state,
		Verifier: flow.Verifier,
	})
}

// Callback 消费 state，完成 code 换 token 和用户资料拉取。
func (m *Manager) Callback(ctx context.Context, source string, code string, state string) (*CallbackResult, error) {
	provider, ok := m.providers[source]
	if !ok {
		return nil, ErrProviderNotEnabled
	}

	flow, err := m.store.Consume(ctx, state)
	if err != nil {
		return nil, err
	}
	if flow.Source != source {
		return nil, ErrStateSourceMismatch
	}

	token, err := provider.Exchange(ctx, code, flow.Verifier)
	if err != nil {
		return nil, err
	}
	profile, err := provider.FetchProfile(ctx, token)
	if err != nil {
		return nil, err
	}
	if profile.Source == "" {
		profile.Source = source
	}

	return &CallbackResult{
		Flow:    flow,
		Token:   token,
		Profile: profile,
	}, nil
}
