package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/service"
)

// stubUserCacheService 最小实现 UserCacheService，用于测试 UserService 委托行为。
type stubUserCacheService struct {
	profile *dto.UserDetailResp
	err     error
}

func (s *stubUserCacheService) Get(_ context.Context, _ int64) (*dto.UserDetailResp, error) {
	return s.profile, s.err
}
func (s *stubUserCacheService) Set(_ context.Context, _ int64, _ *dto.UserDetailResp) error {
	return nil
}
func (s *stubUserCacheService) Invalidate(_ context.Context, _ int64) error { return nil }

func TestUserService_GetDetail_DelegatesToCache(t *testing.T) {
	nickname := "Alice"
	expected := &dto.UserDetailResp{
		ID:       7,
		Username: "alice",
		Nickname: &nickname,
		Roles:    []string{"ROLE_NORMAL", "ROLE_VIP"},
	}
	svc := service.NewUserService(&stubUserCacheService{profile: expected}, nil, nil)

	resp, err := svc.GetDetail(7)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint(7), resp.ID)
	assert.Equal(t, []string{"ROLE_NORMAL", "ROLE_VIP"}, resp.Roles)
}

func TestUserService_GetDetail_PropagatesNotFound(t *testing.T) {
	svc := service.NewUserService(&stubUserCacheService{err: service.ErrUserNotFound}, nil, nil)
	resp, err := svc.GetDetail(9)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, service.ErrUserNotFound)
}
