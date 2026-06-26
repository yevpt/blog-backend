package analytics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/internal/model"
)

const onlineKey = "analytics:online"

// visitorSeenTTL 新访客判定窗口：一年内未出现视为新访客，与 visitor_id cookie 寿命对齐。
const visitorSeenTTL = 365 * 24 * time.Hour

// TodayStat 今日实时计数快照。
type TodayStat struct {
	PV, UV, RegisteredPV, RegisteredUV, AnonymousPV, AnonymousUV int64
}

// Realtime 维护在线人数与今日三档计数（Redis）。
type Realtime interface {
	TouchOnline(ctx context.Context, visitorID string) error
	OnlineCount(ctx context.Context) (int64, error)
	IncrToday(ctx context.Context, ev model.AnalyticsEvent) error
	TodayCounters(ctx context.Context) (TodayStat, error)
	MarkVisitorSeen(ctx context.Context, visitorID string) (bool, error)
}

type realtime struct {
	rdb          *redis.Client
	tz           *time.Location
	onlineWindow time.Duration
}

// NewRealtime 构造实时层。tz 决定今日 key 的切天口径，onlineWindow 为在线判定时间窗。
func NewRealtime(rdb *redis.Client, tz *time.Location, onlineWindow time.Duration) Realtime {
	return &realtime{rdb: rdb, tz: tz, onlineWindow: onlineWindow}
}

func (r *realtime) today() string { return time.Now().In(r.tz).Format("20060102") }

func (r *realtime) ttl() time.Duration {
	now := time.Now().In(r.tz)
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, r.tz).Add(26 * time.Hour)
	return time.Until(next)
}

func (r *realtime) TouchOnline(ctx context.Context, visitorID string) error {
	now := time.Now().Unix()
	if err := r.rdb.ZAdd(ctx, onlineKey, redis.Z{Score: float64(now), Member: visitorID}).Err(); err != nil {
		return fmt.Errorf("在线表写入失败: %w", err)
	}
	return nil
}

func (r *realtime) OnlineCount(ctx context.Context) (int64, error) {
	min := float64(time.Now().Add(-r.onlineWindow).Unix())
	n, err := r.rdb.ZCount(ctx, onlineKey, strconv.FormatFloat(min, 'f', 0, 64), "+inf").Result()
	if err != nil {
		return 0, fmt.Errorf("在线数统计失败: %w", err)
	}
	return n, nil
}

func (r *realtime) IncrToday(ctx context.Context, ev model.AnalyticsEvent) error {
	day := r.today()
	identity := ev.VisitorID
	if ev.IsAuthenticated && ev.UserID != nil {
		identity = "u:" + strconv.FormatUint(uint64(*ev.UserID), 10)
	}
	pipe := r.rdb.Pipeline()
	pipe.Incr(ctx, "analytics:pv:"+day)
	pipe.SAdd(ctx, "analytics:uv:"+day, identity)
	if ev.IsAuthenticated && ev.UserID != nil {
		pipe.Incr(ctx, "analytics:pv:"+day+":registered")
		pipe.SAdd(ctx, "analytics:uv:"+day+":registered", identity)
	} else {
		pipe.Incr(ctx, "analytics:pv:"+day+":anonymous")
		pipe.SAdd(ctx, "analytics:uv:"+day+":anonymous", ev.VisitorID)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("今日计数写入失败: %w", err)
	}
	// 统一设 TTL（到次日 + 缓冲）
	ttl := r.ttl()
	for _, suffix := range []string{"", ":registered", ":anonymous"} {
		r.rdb.Expire(ctx, "analytics:pv:"+day+suffix, ttl)
		r.rdb.Expire(ctx, "analytics:uv:"+day+suffix, ttl)
	}
	return nil
}

// MarkVisitorSeen 以 SetNX 记录访客首次出现：返回 true 表示窗口内首见（新访客）。
func (r *realtime) MarkVisitorSeen(ctx context.Context, visitorID string) (bool, error) {
	first, err := r.rdb.SetNX(ctx, "analytics:visitor:seen:"+visitorID, 1, visitorSeenTTL).Result()
	if err != nil {
		return false, fmt.Errorf("新访客判定写入失败: %w", err)
	}
	return first, nil
}

func (r *realtime) TodayCounters(ctx context.Context) (TodayStat, error) {
	day := r.today()
	pv, _ := r.rdb.Get(ctx, "analytics:pv:"+day).Int64()
	uv, _ := r.rdb.SCard(ctx, "analytics:uv:"+day).Result()
	rpv, _ := r.rdb.Get(ctx, "analytics:pv:"+day+":registered").Int64()
	ruv, _ := r.rdb.SCard(ctx, "analytics:uv:"+day+":registered").Result()
	apv, _ := r.rdb.Get(ctx, "analytics:pv:"+day+":anonymous").Int64()
	auv, _ := r.rdb.SCard(ctx, "analytics:uv:"+day+":anonymous").Result()
	return TodayStat{PV: pv, UV: uv, RegisteredPV: rpv, RegisteredUV: ruv, AnonymousPV: apv, AnonymousUV: auv}, nil
}
