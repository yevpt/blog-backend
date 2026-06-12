package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/internal/service"
)

// stubUserRepo 最小实现 UserRepository，仅 FindDetailByID 返回预设值
type stubUserRepo struct {
	aggregate *repository.UserDetailAggregate
	err       error
}

func (r *stubUserRepo) FindDetailByID(id uint) (*repository.UserDetailAggregate, error) {
	return r.aggregate, r.err
}

// 接口其余方法返回零值，测试中不会被调用
func (r *stubUserRepo) FindByIdentifier(id string) (*model.User, error) { return nil, nil }
func (r *stubUserRepo) FindByID(id uint) (*model.User, error)           { return nil, nil }
func (r *stubUserRepo) Create(u *model.User, roleID uint) error         { return nil }
func (r *stubUserRepo) ExistsByEmail(email string) (bool, error)        { return false, nil }
func (r *stubUserRepo) ExistsByNickname(n string) (bool, error)         { return false, nil }
func (r *stubUserRepo) FindRolesByUserID(id uint) ([]string, error)     { return nil, nil }
func (r *stubUserRepo) FindRolesByUserIDs(ids []uint) (map[uint][]string, error) {
	return nil, nil
}
func (r *stubUserRepo) UpdateLastLoginAt(id uint) error { return nil }
func (r *stubUserRepo) ListRecent(offset, limit int) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *stubUserRepo) ListAll(offset, limit int) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *stubUserRepo) Update(id uint, updates map[string]interface{}) error { return nil }

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestUserCacheService_Get_CacheMiss_ThenHit(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()

	nickname := "alice"
	stub := &stubUserRepo{
		aggregate: &repository.UserDetailAggregate{
			User:  model.User{Base: model.Base{ID: 1}, Username: "alice", Nickname: &nickname, Status: 1},
			Roles: []string{"ROLE_NORMAL"},
		},
	}
	svc := service.NewUserCacheService(stub, nil, rdb)

	// 第一次：cache miss，从 DB 读取
	profile, err := svc.Get(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "alice", profile.Username)
	assert.Equal(t, []string{"ROLE_NORMAL"}, profile.Roles)

	// 验证缓存已写入
	cached, err := rdb.Get(ctx, "user:profile:1").Result()
	require.NoError(t, err)
	assert.NotEmpty(t, cached)

	// 第二次：cache hit（stub 设为 nil，确认不再查 DB）
	stub.aggregate = nil
	profile2, err := svc.Get(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "alice", profile2.Username)
}

func TestUserCacheService_Get_UserNotFound(t *testing.T) {
	rdb := newTestRedis(t)
	svc := service.NewUserCacheService(&stubUserRepo{aggregate: nil}, nil, rdb)

	_, err := svc.Get(context.Background(), 99)
	assert.ErrorIs(t, err, service.ErrUserNotFound)
}

func TestUserCacheService_Set_And_Invalidate(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()
	svc := service.NewUserCacheService(&stubUserRepo{}, nil, rdb)

	profile := &dto.UserDetailResp{ID: 5, Username: "bob", Roles: []string{"ROLE_VIP"}}
	require.NoError(t, svc.Set(ctx, 5, profile))

	// 验证可以读回
	got, err := svc.Get(ctx, 5)
	require.NoError(t, err)
	assert.Equal(t, "bob", got.Username)

	// Invalidate 后 Get 走 DB（stub 返回 nil → ErrUserNotFound）
	require.NoError(t, svc.Invalidate(ctx, 5))
	_, err = svc.Get(ctx, 5)
	assert.ErrorIs(t, err, service.ErrUserNotFound)
}

func TestUserCacheService_Get_CorruptJSON_Rebuilds(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()

	rdb.Set(ctx, "user:profile:3", "not-valid-json", 7*24*time.Hour)

	nickname := "carol"
	stub := &stubUserRepo{
		aggregate: &repository.UserDetailAggregate{
			User:  model.User{Base: model.Base{ID: 3}, Username: "carol", Nickname: &nickname, Status: 1},
			Roles: []string{"ROLE_NORMAL"},
		},
	}
	svc := service.NewUserCacheService(stub, nil, rdb)

	profile, err := svc.Get(ctx, 3)
	require.NoError(t, err)
	assert.Equal(t, "carol", profile.Username)

	cached, _ := rdb.Get(ctx, "user:profile:3").Result()
	var rebuilt dto.UserDetailResp
	assert.NoError(t, json.Unmarshal([]byte(cached), &rebuilt))
}

func TestUserCacheService_Get_RedisError_ReturnsError(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// 关闭 miniredis 模拟 Redis 故障
	mr.Close()

	svc := service.NewUserCacheService(&stubUserRepo{}, nil, rdb)
	_, err = svc.Get(context.Background(), 1)
	assert.Error(t, err)
	// 确认不是 ErrUserNotFound（DB 没被调用）
	assert.NotErrorIs(t, err, service.ErrUserNotFound)
}
