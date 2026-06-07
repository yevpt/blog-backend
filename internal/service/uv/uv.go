package uv

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// UVService 通用 UV 去重服务，基于 Redis SET NX EX 实现。
// 同一 visitor 对同一资源在 window 时间窗口内只计一次。
type UVService interface {
	// CheckAndMark 检查并标记 UV。
	// prefix: 业务前缀，如 "article:viewed"、"moment:viewed"
	// targetID: 资源 ID
	// visitorID: 访客标识
	// window: 去重时间窗口
	// 返回 true 表示新访客（应计入），false 表示已计过。
	CheckAndMark(ctx context.Context, prefix, targetID, visitorID string, window time.Duration) (bool, error)
}

type uvService struct {
	rdb *redis.Client
}

// NewService 创建 UV 去重服务实例。
func NewService(rdb *redis.Client) UVService {
	return &uvService{rdb: rdb}
}

func (s *uvService) CheckAndMark(ctx context.Context, prefix, targetID, visitorID string, window time.Duration) (bool, error) {
	key := fmt.Sprintf("%s:%s:visitor:%s", prefix, targetID, visitorID)
	ok, err := s.rdb.SetNX(ctx, key, 1, window).Result()
	if err != nil {
		return false, fmt.Errorf("UV 去重写入失败: %w", err)
	}
	return ok, nil
}