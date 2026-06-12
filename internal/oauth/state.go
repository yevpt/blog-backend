package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const stateKeyPrefix = "oauth:state:"

// FlowContext 是一次 OAuth 授权开始时写入 Redis 的流程上下文。
type FlowContext struct {
	State       string    `json:"state"`        // 随机 state，用于 CSRF 防护和 callback 流程匹配
	Source      string    `json:"source"`       // 第三方平台标识
	Action      Action    `json:"action"`       // login 或 bind
	UserID      uint      `json:"user_id"`      // 绑定流程中的当前登录用户 ID，登录流程为 0
	Verifier    string    `json:"verifier"`     // PKCE code verifier，callback 换 token 时使用
	RedirectURI string    `json:"redirect_uri"` // 前端回跳地址，后续如需 302 时使用
	CreatedAt   time.Time `json:"created_at"`   // 创建时间，用于排查授权链路问题
}

// StateStore 负责保存和一次性消费 OAuth state。
type StateStore struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewStateStore 创建 OAuth state 存储器。
func NewStateStore(rdb *redis.Client, ttl time.Duration) *StateStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &StateStore{rdb: rdb, ttl: ttl}
}

// Create 生成随机 state，并把流程上下文写入 Redis。
func (s *StateStore) Create(ctx context.Context, flow FlowContext) (string, error) {
	state, err := randomState()
	if err != nil {
		return "", err
	}

	// state 由后端生成并写回上下文，避免信任任何前端传入的流程标识。
	flow.State = state
	flow.CreatedAt = time.Now()

	payload, err := json.Marshal(flow)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, stateKey(state), payload, s.ttl).Err(); err != nil {
		return "", err
	}
	return state, nil
}

// Consume 读取并删除 state，确保同一个 callback 不能被重放。
func (s *StateStore) Consume(ctx context.Context, state string) (*FlowContext, error) {
	payload, err := s.rdb.GetDel(ctx, stateKey(state)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrInvalidState
	}
	if err != nil {
		return nil, err
	}

	var flow FlowContext
	if err := json.Unmarshal([]byte(payload), &flow); err != nil {
		return nil, fmt.Errorf("解析 OAuth state 失败: %w", err)
	}
	return &flow, nil
}

func randomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func stateKey(state string) string {
	return stateKeyPrefix + state
}
