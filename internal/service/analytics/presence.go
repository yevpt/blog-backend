package analytics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// profileCacheInvalidator 用户资料缓存失效（避免 analytics → user 循环依赖）。
type profileCacheInvalidator interface {
	Invalidate(ctx context.Context, userId int64) error
}

const (
	// UserOnlineKey 注册用户在线 ZSET，member=user_id，score=最后活跃 unix 秒。
	UserOnlineKey         = "user:online"
	presenceDBTouchPrefix = "user:presence:db_touch:"
	presenceDBTouchTTL    = 60 * time.Second
)

// UserPresence 维护注册用户在线态（Redis）与 DB last_active_at（节流）。
type UserPresence interface {
	TouchUserOnline(ctx context.Context, userID uint) error
	IsUserOnline(ctx context.Context, userID uint) (bool, error)
	BatchIsUserOnline(ctx context.Context, userIDs []uint) (map[uint]bool, error)
	TouchActiveAt(ctx context.Context, userID uint) error
}

// UserActiveAtRepository presence 写 last_active_at 所需的最小 repo 面。
type UserActiveAtRepository interface {
	UpdateLastActiveAt(userID uint) error
}

// lastActiveAtUpdater 为 UserActiveAtRepository 别名，供包内使用。
type lastActiveAtUpdater = UserActiveAtRepository

type userPresence struct {
	rdb          *redis.Client
	repo         lastActiveAtUpdater
	cache        profileCacheInvalidator
	onlineWindow time.Duration
}

// NewUserPresence 构造用户 presence 服务。onlineWindow 与 analytics.online_window 一致。
func NewUserPresence(
	rdb *redis.Client,
	repo UserActiveAtRepository,
	cache profileCacheInvalidator,
	onlineWindow time.Duration,
) UserPresence {
	return &userPresence{rdb: rdb, repo: repo, cache: cache, onlineWindow: onlineWindow}
}

func (p *userPresence) TouchUserOnline(ctx context.Context, userID uint) error {
	now := time.Now().Unix()
	member := strconv.FormatUint(uint64(userID), 10)
	if err := p.rdb.ZAdd(ctx, UserOnlineKey, redis.Z{Score: float64(now), Member: member}).Err(); err != nil {
		return fmt.Errorf("user online zadd: %w", err)
	}
	return nil
}

func (p *userPresence) IsUserOnline(ctx context.Context, userID uint) (bool, error) {
	m, err := p.BatchIsUserOnline(ctx, []uint{userID})
	if err != nil {
		return false, err
	}
	return m[userID], nil
}

func (p *userPresence) BatchIsUserOnline(ctx context.Context, userIDs []uint) (map[uint]bool, error) {
	out := make(map[uint]bool, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	minScore := float64(time.Now().Add(-p.onlineWindow).Unix())
	pipe := p.rdb.Pipeline()
	cmds := make([]*redis.FloatCmd, len(userIDs))
	for i, id := range userIDs {
		cmds[i] = pipe.ZScore(ctx, UserOnlineKey, strconv.FormatUint(uint64(id), 10))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("user online batch zscore: %w", err)
	}
	for i, id := range userIDs {
		score, err := cmds[i].Result()
		if err == redis.Nil {
			out[id] = false
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("user online zscore uid=%d: %w", id, err)
		}
		out[id] = score >= minScore
	}
	return out, nil
}

func (p *userPresence) TouchActiveAt(ctx context.Context, userID uint) error {
	lockKey := presenceDBTouchPrefix + strconv.FormatUint(uint64(userID), 10)
	ok, err := p.rdb.SetNX(ctx, lockKey, "1", presenceDBTouchTTL).Result()
	if err != nil {
		return fmt.Errorf("presence db touch lock: %w", err)
	}
	if !ok {
		return nil
	}
	if err := p.repo.UpdateLastActiveAt(userID); err != nil {
		return err
	}
	if p.cache != nil {
		_ = p.cache.Invalidate(ctx, int64(userID))
	}
	return nil
}
