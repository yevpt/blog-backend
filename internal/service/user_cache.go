package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/pkg/storage"
)

const userCacheTTL = 7 * 24 * time.Hour

// UserCacheService 管理用户资料的 Redis 缓存，对调用方屏蔽 DB/缓存细节。
type UserCacheService interface {
	// Get 优先从 Redis 读取（GETEX 刷新 TTL）；未命中时查 DB 并回填缓存。
	Get(ctx context.Context, userId int64) (*dto.UserDetailResp, error)
	// Set 写入缓存（登录时主动预热，或用户信息更新后重建）。
	Set(ctx context.Context, userId int64, profile *dto.UserDetailResp) error
	// Invalidate 删除缓存（用户信息变更时调用，下次 Get 自动重建）。
	Invalidate(ctx context.Context, userId int64) error
}

type userCacheService struct {
	repo     repository.UserRepository
	resolver storage.ObjectURLResolver
	rdb      *redis.Client
}

func NewUserCacheService(
	repo repository.UserRepository,
	resolver storage.ObjectURLResolver,
	rdb *redis.Client,
) UserCacheService {
	return &userCacheService{repo: repo, resolver: resolver, rdb: rdb}
}

func userCacheKey(userId int64) string {
	return fmt.Sprintf("user:profile:%d", userId)
}

func (s *userCacheService) Get(ctx context.Context, userId int64) (*dto.UserDetailResp, error) {
	key := userCacheKey(userId)
	// GETEX 原子地读取并将 TTL 重置为 7 天（Redis 6.2+；go-redis v9 支持）
	val, err := s.rdb.GetEx(ctx, key, userCacheTTL).Result()
	if err == nil {
		var profile dto.UserDetailResp
		if jsonErr := json.Unmarshal([]byte(val), &profile); jsonErr == nil {
			return &profile, nil
		}
		// JSON 损坏，删掉强制重建
		s.rdb.Del(ctx, key)
	}

	// Cache miss：查询 DB，组装 DTO，回填缓存
	aggregate, dbErr := s.repo.FindDetailByID(uint(userId))
	if dbErr != nil {
		return nil, dbErr
	}
	if aggregate == nil {
		return nil, ErrUserNotFound
	}

	profile := assembleUserDetail(s.resolver, aggregate)
	_ = s.Set(ctx, userId, profile) // 写缓存失败不影响返回
	return profile, nil
}

func (s *userCacheService) Set(ctx context.Context, userId int64, profile *dto.UserDetailResp) error {
	data, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, userCacheKey(userId), string(data), userCacheTTL).Err()
}

func (s *userCacheService) Invalidate(ctx context.Context, userId int64) error {
	return s.rdb.Del(ctx, userCacheKey(userId)).Err()
}
