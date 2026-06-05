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
	return s.cache.Get(context.Background(), int64(userID))
}
