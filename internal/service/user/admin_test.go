package user_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/model"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	"github.com/vpt/blog-backend/internal/repository/user/mock"
	user "github.com/vpt/blog-backend/internal/service/user"
	usermock "github.com/vpt/blog-backend/internal/service/user/mock"
	"github.com/vpt/blog-backend/pkg/roles"
)

func TestAdminService_GrantVip_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	cache := &stubUserCacheService{}

	repo.EXPECT().FindByID(uint(7)).Return(&model.User{Base: model.Base{ID: 7}}, nil)
	repo.EXPECT().GrantVipRole(uint(7)).Return(nil)
	repo.EXPECT().FindRolesByUserID(uint(7)).Return([]string{roles.NormalRole, roles.VipRole}, nil)

	svc := user.NewAdminService(repo, cache)
	resp, err := svc.GrantVip(7)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint(7), resp.UserID)
	assert.Equal(t, []string{roles.NormalRole, roles.VipRole}, resp.Roles)
}

func TestAdminService_GetAdminDetail_FillsIsOnlineFromPresence(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	presence := usermock.NewMockOnlineChecker(ctrl)

	repo.EXPECT().FindDetailByID(uint(5)).Return(&userrepo.UserDetailAggregate{
		User: model.User{Base: model.Base{ID: 5}, Username: "vpt"},
	}, nil)
	presence.EXPECT().IsUserOnline(gomock.Any(), uint(5)).Return(true, nil)

	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Presence: presence,
	})
	resp, err := svc.GetAdminDetail(5)
	require.NoError(t, err)
	assert.True(t, resp.IsOnline)
}

func TestAdminService_RevokeVip_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	cache := &stubUserCacheService{}

	repo.EXPECT().FindByID(uint(7)).Return(&model.User{Base: model.Base{ID: 7}}, nil)
	repo.EXPECT().RevokeVipRole(uint(7)).Return(nil)
	repo.EXPECT().FindRolesByUserID(uint(7)).Return([]string{roles.NormalRole}, nil)

	svc := user.NewAdminService(repo, cache)
	resp, err := svc.RevokeVip(7)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, []string{roles.NormalRole}, resp.Roles)
}

func TestAdminService_GrantVip_UserNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)

	repo.EXPECT().FindByID(uint(9)).Return(nil, nil)

	svc := user.NewAdminService(repo, &stubUserCacheService{})
	resp, err := svc.GrantVip(9)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, user.ErrUserNotFound)
}
