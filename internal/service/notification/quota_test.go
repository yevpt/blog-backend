package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// fakeQuotaStore 用内存策略与用量驱动额度评估。
type fakeQuotaStore struct {
	policies   []model.EmailQuotaPolicy
	roleQuotas []model.EmailRoleQuotaPolicy
	usage      map[string]int // scope|scopeID|purpose|window
}

func (f *fakeQuotaStore) GetQuotaPolicies(context.Context) ([]model.EmailQuotaPolicy, error) {
	return f.policies, nil
}
func (f *fakeQuotaStore) GetRoleQuotaPolicies(context.Context) ([]model.EmailRoleQuotaPolicy, error) {
	return f.roleQuotas, nil
}
func (f *fakeQuotaStore) GetUsage(_ context.Context, key notificationrepo.QuotaUsageKey) (int, error) {
	return f.usage[usageKey(key.ScopeType, key.ScopeID, key.Purpose, key.WindowType)], nil
}
func (f *fakeQuotaStore) ReserveQuota(_ context.Context, key notificationrepo.QuotaUsageKey, limit int) (bool, error) {
	if f.usage == nil {
		f.usage = map[string]int{}
	}
	k := usageKey(key.ScopeType, key.ScopeID, key.Purpose, key.WindowType)
	if f.usage[k]+1 > limit {
		return false, nil
	}
	f.usage[k]++
	return true, nil
}

func usageKey(scope string, id uint, purpose, window string) string {
	return scope + "|" + string(rune(id)) + "|" + purpose + "|" + window
}

func defaultPolicies() []model.EmailQuotaPolicy {
	return []model.EmailQuotaPolicy{
		{Purpose: "register_code", DailyLimit: 100, ReservedMin: 50, MaxPerMinute: 0, MaxPerHour: 0, Enabled: true},
		{Purpose: "password_reset", DailyLimit: 100, ReservedMin: 30, Enabled: true},
		{Purpose: "notification", DailyLimit: 150, ReservedMin: 0, Enabled: true},
	}
}

func cfg() notificationservice.QuotaConfig {
	return notificationservice.QuotaConfig{SiteDailySafeLimit: 300, MaxPerMinute: 5, MaxPerHour: 80}
}

func now() time.Time { return time.Date(2026, 6, 23, 10, 30, 0, 0, time.Local) }

// register_code 有保底份额：即使全站接近上限，仍可发送。
func TestQuota_RegisterCodeHasReservedQuota(t *testing.T) {
	store := &fakeQuotaStore{
		policies: defaultPolicies(),
		// 全站日用量 270：notification 的有效额度为 300-80=220 已被挤爆，但 register_code 有效额度为 300-30=270。
		usage: map[string]int{usageKey("site", 0, "*", "day"): 269},
	}
	svc := notificationservice.NewQuotaService(store, cfg())

	d, err := svc.Evaluate(context.Background(), notificationservice.QuotaInput{Purpose: "register_code", Now: now()})

	require.NoError(t, err)
	assert.True(t, d.Allowed)
}

// notification 不能占用为 password_reset 等预留的全站额度。
func TestQuota_NotificationCannotConsumeReserved(t *testing.T) {
	store := &fakeQuotaStore{
		policies: defaultPolicies(),
		usage:    map[string]int{usageKey("site", 0, "*", "day"): 220},
	}
	svc := notificationservice.NewQuotaService(store, cfg())

	d, err := svc.Evaluate(context.Background(), notificationservice.QuotaInput{Purpose: "notification", Now: now()})

	require.NoError(t, err)
	assert.False(t, d.Allowed)
	assert.Equal(t, "全站每日安全额度不足", d.Reason)
	assert.True(t, d.DeferUntil.After(now()))
}

// 操作人每日额度用尽时延后。
func TestQuota_ActorDailyLimitDefers(t *testing.T) {
	store := &fakeQuotaStore{
		policies:   defaultPolicies(),
		roleQuotas: []model.EmailRoleQuotaPolicy{{Role: "normal", ScopeType: "actor", DailyLimit: 30, Enabled: true}},
		usage:      map[string]int{usageKey("actor", 7, "*", "day"): 30},
	}
	svc := notificationservice.NewQuotaService(store, cfg())

	d, err := svc.Evaluate(context.Background(), notificationservice.QuotaInput{
		Purpose: "notification", ActorUserID: 7, ActorRoles: []string{"normal"}, Now: now(),
	})

	require.NoError(t, err)
	assert.False(t, d.Allowed)
	assert.Equal(t, "操作人每日通知邮件额度不足", d.Reason)
}

// 接收人每日额度用尽时延后。
func TestQuota_RecipientDailyLimitDefers(t *testing.T) {
	store := &fakeQuotaStore{
		policies:   defaultPolicies(),
		roleQuotas: []model.EmailRoleQuotaPolicy{{Role: "normal", ScopeType: "recipient", DailyLimit: 5, Enabled: true}},
		usage:      map[string]int{usageKey("recipient", 9, "*", "day"): 5},
	}
	svc := notificationservice.NewQuotaService(store, cfg())

	d, err := svc.Evaluate(context.Background(), notificationservice.QuotaInput{
		Purpose: "notification", RecipientUserID: 9, RecipientRoles: []string{"normal"}, Now: now(),
	})

	require.NoError(t, err)
	assert.False(t, d.Allowed)
	assert.Equal(t, "接收人每日通知邮件额度不足", d.Reason)
}

// 管理员也受限额约束。
func TestQuota_AdminStillHasLimits(t *testing.T) {
	store := &fakeQuotaStore{
		policies:   defaultPolicies(),
		roleQuotas: []model.EmailRoleQuotaPolicy{{Role: "admin", ScopeType: "actor", DailyLimit: 300, Enabled: true}},
		usage:      map[string]int{usageKey("actor", 1, "*", "day"): 300},
	}
	svc := notificationservice.NewQuotaService(store, cfg())

	d, err := svc.Evaluate(context.Background(), notificationservice.QuotaInput{
		Purpose: "notification", ActorUserID: 1, ActorRoles: []string{"admin"}, Now: now(),
	})

	require.NoError(t, err)
	assert.False(t, d.Allowed)
	assert.Equal(t, "操作人每日通知邮件额度不足", d.Reason)
}

// Reserve 在额度充足时占用成功并自增用量，达到上限时拒绝。
func TestQuota_ReserveConsumesAndRejects(t *testing.T) {
	store := &fakeQuotaStore{
		policies:   defaultPolicies(),
		roleQuotas: []model.EmailRoleQuotaPolicy{{Role: "normal", ScopeType: "actor", DailyLimit: 1, Enabled: true}},
		usage:      map[string]int{},
	}
	svc := notificationservice.NewQuotaService(store, cfg())
	in := notificationservice.QuotaInput{Purpose: "notification", ActorUserID: 7, ActorRoles: []string{"normal"}, Now: now()}

	// 首次占用成功。
	d, err := svc.Reserve(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, d.Allowed)

	// actor 日额度为 1，第二次占用被拒绝。
	d, err = svc.Reserve(context.Background(), in)
	require.NoError(t, err)
	assert.False(t, d.Allowed)
	assert.Equal(t, "操作人每日通知邮件额度不足", d.Reason)
}

// 全站每分钟/每小时频率上限被强制执行。
func TestQuota_ProviderRateLimitsEnforced(t *testing.T) {
	t.Run("每分钟上限", func(t *testing.T) {
		store := &fakeQuotaStore{
			policies: defaultPolicies(),
			usage:    map[string]int{usageKey("site", 0, "*", "minute"): 5},
		}
		svc := notificationservice.NewQuotaService(store, cfg())

		d, err := svc.Evaluate(context.Background(), notificationservice.QuotaInput{Purpose: "notification", Now: now()})

		require.NoError(t, err)
		assert.False(t, d.Allowed)
		assert.Equal(t, "全站每分钟发送上限", d.Reason)
	})

	t.Run("每小时上限", func(t *testing.T) {
		store := &fakeQuotaStore{
			policies: defaultPolicies(),
			usage:    map[string]int{usageKey("site", 0, "*", "hour"): 80},
		}
		svc := notificationservice.NewQuotaService(store, cfg())

		d, err := svc.Evaluate(context.Background(), notificationservice.QuotaInput{Purpose: "notification", Now: now()})

		require.NoError(t, err)
		assert.False(t, d.Allowed)
		assert.Equal(t, "全站每小时发送上限", d.Reason)
	})
}
