package user_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository/user/mock"
	user "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/roles"
)

func TestAdminService_DisableAccount_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	cache := &stubUserCacheService{}

	repo.EXPECT().FindByID(uint(7)).Return(&model.User{Base: model.Base{ID: 7}}, nil)
	repo.EXPECT().FindRolesByUserID(uint(7)).Return([]string{roles.NormalRole}, nil)
	repo.EXPECT().SetStatus(uint(7), uint8(0)).Return(nil)

	svc := user.NewAdminService(repo, cache)
	err := svc.DisableAccount(1, 7)
	require.NoError(t, err)
}

func TestAdminService_DisableAccount_RejectsSelf(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)

	svc := user.NewAdminService(repo, &stubUserCacheService{})
	err := svc.DisableAccount(7, 7)
	assert.ErrorIs(t, err, user.ErrCannotDisableSelf)
}

func TestAdminService_DisableAccount_RejectsLastAdmin(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)

	repo.EXPECT().FindByID(uint(9)).Return(&model.User{Base: model.Base{ID: 9}}, nil)
	repo.EXPECT().FindRolesByUserID(uint(9)).Return([]string{roles.AdminRole}, nil)
	repo.EXPECT().CountByRole(roles.AdminRole).Return(int64(1), nil)

	svc := user.NewAdminService(repo, &stubUserCacheService{})
	err := svc.DisableAccount(1, 9)
	assert.ErrorIs(t, err, user.ErrLastAdminAccount)
}

func TestAdminService_EnableAccount_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)

	repo.EXPECT().FindByID(uint(7)).Return(&model.User{Base: model.Base{ID: 7}}, nil)
	repo.EXPECT().SetStatus(uint(7), uint8(1)).Return(nil)

	svc := user.NewAdminService(repo, &stubUserCacheService{})
	err := svc.EnableAccount(7)
	require.NoError(t, err)
}
