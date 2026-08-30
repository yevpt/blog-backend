package user_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository/user"
	"github.com/vpt/blog-backend/internal/repository/user/mock"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	usermock "github.com/vpt/blog-backend/internal/service/user/mock"
)

type stubModerationReader struct{ state string }

func (s *stubModerationReader) GetSanctionState(uint) (string, error) { return s.state, nil }

func TestAdminService_ListAdmin_PassesFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)

	repo.EXPECT().
		ListAll(user.UserListFilter{Keyword: "vpt", Role: "ROLE_ADMIN"}, 0, 10).
		Return([]model.User{{Base: model.Base{ID: 1}, Username: "vpt"}}, int64(1), nil)
	repo.EXPECT().FindRolesByUserIDs([]uint{1}).Return(map[uint][]string{1: {"ROLE_ADMIN"}}, nil)

	svc := userservice.NewAdminService(repo, &stubUserCacheService{}, userservice.AdminDeps{
		Moderation: &stubModerationReader{state: "active"},
	})
	resp, err := svc.ListAdmin(&dto.AdminUserListReq{Page: 1, PageSize: 10, Keyword: "vpt", Role: "ROLE_ADMIN"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Total)
	assert.Equal(t, "active", resp.List[0].SanctionState)
}

func TestAdminService_ListAdmin_FillsIsOnlineFromPresence(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	presence := usermock.NewMockOnlineChecker(ctrl)

	repo.EXPECT().
		ListAll(user.UserListFilter{}, 0, 10).
		Return([]model.User{
			{Base: model.Base{ID: 1}, Username: "vpt"},
			{Base: model.Base{ID: 2}, Username: "foo"},
		}, int64(2), nil)
	repo.EXPECT().FindRolesByUserIDs([]uint{1, 2}).Return(map[uint][]string{}, nil)
	presence.EXPECT().
		BatchIsUserOnline(gomock.Any(), []uint{1, 2}).
		Return(map[uint]bool{1: true}, nil)

	svc := userservice.NewAdminService(repo, &stubUserCacheService{}, userservice.AdminDeps{
		Presence: presence,
	})
	resp, err := svc.ListAdmin(&dto.AdminUserListReq{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, resp.List, 2)
	assert.True(t, resp.List[0].IsOnline)
	assert.False(t, resp.List[1].IsOnline)
}

func TestAdminService_ListAdmin_NoModerationDepsDefaultsActive(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)

	repo.EXPECT().
		ListAll(user.UserListFilter{}, 0, 10).
		Return([]model.User{{Base: model.Base{ID: 2}, Username: "foo"}}, int64(1), nil)
	repo.EXPECT().FindRolesByUserIDs([]uint{2}).Return(map[uint][]string{}, nil)

	svc := userservice.NewAdminService(repo, &stubUserCacheService{})
	resp, err := svc.ListAdmin(&dto.AdminUserListReq{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, "active", resp.List[0].SanctionState)
}

func TestAdminService_GetAdminDetail_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	repo.EXPECT().FindDetailByID(gomock.Any(), uint(9)).Return(nil, nil)

	svc := userservice.NewAdminService(repo, &stubUserCacheService{})
	resp, err := svc.GetAdminDetail(9)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, userservice.ErrUserNotFound)
}

func TestAdminService_GetAdminDetail_Found(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	repo.EXPECT().FindDetailByID(gomock.Any(), uint(3)).Return(&user.UserDetailAggregate{
		User:  model.User{Base: model.Base{ID: 3}, Username: "vpt"},
		Roles: []string{"ROLE_ADMIN"},
	}, nil)

	svc := userservice.NewAdminService(repo, &stubUserCacheService{}, userservice.AdminDeps{
		Moderation: &stubModerationReader{state: "muted"},
	})
	resp, err := svc.GetAdminDetail(3)
	require.NoError(t, err)
	assert.Equal(t, uint(3), resp.ID)
	assert.Equal(t, "vpt", resp.Username)
	assert.Equal(t, []string{"ROLE_ADMIN"}, resp.Roles)
	assert.Equal(t, "muted", resp.SanctionState)
}
