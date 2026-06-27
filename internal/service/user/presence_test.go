package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	repomock "github.com/vpt/blog-backend/internal/repository/user/mock"
	user "github.com/vpt/blog-backend/internal/service/user"
	usermock "github.com/vpt/blog-backend/internal/service/user/mock"
)

func TestPresenceProvider_BatchPresence_ZipsOnlineAndActiveLogin(t *testing.T) {
	ctrl := gomock.NewController(t)
	online := usermock.NewMockOnlineChecker(ctrl)
	repo := repomock.NewMockUserRepository(ctrl)

	activeAt := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	loginAt := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)

	online.EXPECT().BatchIsUserOnline(gomock.Any(), []uint{1, 2}).Return(map[uint]bool{1: true}, nil)
	repo.EXPECT().BatchFetchActiveLogin([]uint{1, 2}).Return(map[uint]*userrepo.ActiveLogin{
		1: {LastActiveAt: &activeAt, LastLoginAt: &loginAt},
		2: {LastActiveAt: nil, LastLoginAt: nil},
	}, nil)

	provider := user.NewPresenceProvider(online, repo)
	result, err := provider.BatchPresence(context.Background(), []uint{1, 2})
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.True(t, result[1].IsOnline)
	require.NotNil(t, result[1].LastActiveAt)
	assert.Equal(t, activeAt.Unix(), *result[1].LastActiveAt)
	require.NotNil(t, result[1].LastLoginAt)
	assert.Equal(t, loginAt.Unix(), *result[1].LastLoginAt)

	assert.False(t, result[2].IsOnline)
	assert.Nil(t, result[2].LastActiveAt)
	assert.Nil(t, result[2].LastLoginAt)
}

func TestPresenceProvider_BatchPresence_UnknownIDAbsentFromResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	online := usermock.NewMockOnlineChecker(ctrl)
	repo := repomock.NewMockUserRepository(ctrl)

	online.EXPECT().BatchIsUserOnline(gomock.Any(), []uint{99}).Return(map[uint]bool{}, nil)
	repo.EXPECT().BatchFetchActiveLogin([]uint{99}).Return(map[uint]*userrepo.ActiveLogin{}, nil)

	provider := user.NewPresenceProvider(online, repo)
	result, err := provider.BatchPresence(context.Background(), []uint{99})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestPresenceProvider_BatchPresence_OnlineCheckErrorPropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	online := usermock.NewMockOnlineChecker(ctrl)
	repo := repomock.NewMockUserRepository(ctrl)

	wantErr := errors.New("redis down")
	online.EXPECT().BatchIsUserOnline(gomock.Any(), []uint{1}).Return(nil, wantErr)

	provider := user.NewPresenceProvider(online, repo)
	_, err := provider.BatchPresence(context.Background(), []uint{1})
	assert.ErrorIs(t, err, wantErr)
}

func TestPresenceProvider_BatchPresence_RepoErrorPropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	online := usermock.NewMockOnlineChecker(ctrl)
	repo := repomock.NewMockUserRepository(ctrl)

	wantErr := errors.New("db down")
	online.EXPECT().BatchIsUserOnline(gomock.Any(), []uint{1}).Return(map[uint]bool{1: true}, nil)
	repo.EXPECT().BatchFetchActiveLogin([]uint{1}).Return(nil, wantErr)

	provider := user.NewPresenceProvider(online, repo)
	_, err := provider.BatchPresence(context.Background(), []uint{1})
	assert.ErrorIs(t, err, wantErr)
}

func TestPresenceProvider_BatchPresence_EmptyIDsReturnsEmptyMapWithoutCalls(t *testing.T) {
	ctrl := gomock.NewController(t)
	online := usermock.NewMockOnlineChecker(ctrl)
	repo := repomock.NewMockUserRepository(ctrl)
	// 不设置任何 EXPECT：空 ids 不应触碰 Redis/DB

	provider := user.NewPresenceProvider(online, repo)
	result, err := provider.BatchPresence(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}
