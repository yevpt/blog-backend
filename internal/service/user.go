package service

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
)

// UserService 用户资料业务接口。
type UserService interface {
	GetDetail(userID uint) (*dto.UserDetailResp, error)
}

type userService struct {
	cache UserCacheService
}

// NewUserService 创建用户资料服务，依赖 UserCacheService（Redis 优先，DB 兜底）。
func NewUserService(cache UserCacheService) UserService {
	return &userService{cache: cache}
}

func (s *userService) GetDetail(userID uint) (*dto.UserDetailResp, error) {
	// context.Background() 是有意为之：此方法仅供 handler 过渡期使用，
	// Task 4 后 handler 将直接从 gin.Context 读取 UserDetail，此方法弃用。
	return s.cache.Get(context.Background(), int64(userID))
}
