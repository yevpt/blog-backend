package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// plannerRepoStub 记录领取的任务与 planner 的批次/延后/释放调用。
type plannerRepoStub struct {
	leased []model.NotificationEmailTask

	batches    [][]uint // 每个批次包含的任务 ID
	deferred   []uint
	released   []uint
	itemCounts []int
}

func (s *plannerRepoStub) CreateEmailTask(context.Context, *model.NotificationEmailTask) (bool, error) {
	return true, nil
}
func (s *plannerRepoStub) LeaseEmailTasks(context.Context, string, int, int) ([]model.NotificationEmailTask, error) {
	return s.leased, nil
}
func (s *plannerRepoStub) DeferEmailTasks(_ context.Context, ids []uint, _ time.Time) error {
	s.deferred = append(s.deferred, ids...)
	return nil
}
func (s *plannerRepoStub) ReleaseEmailTasks(_ context.Context, ids []uint) error {
	s.released = append(s.released, ids...)
	return nil
}
func (s *plannerRepoStub) CreateEmailBatchWithItems(_ context.Context, batch *model.NotificationEmailBatch, ids []uint) error {
	s.batches = append(s.batches, ids)
	s.itemCounts = append(s.itemCounts, batch.ItemCount)
	return nil
}
func (s *plannerRepoStub) LeaseEmailBatches(context.Context, string, int, int) ([]model.NotificationEmailBatch, error) {
	return nil, nil
}
func (s *plannerRepoStub) MarkBatchSent(context.Context, uint, string) error             { return nil }
func (s *plannerRepoStub) MarkBatchRetry(context.Context, uint, time.Time, string) error { return nil }
func (s *plannerRepoStub) ListBatchTasks(context.Context, uint) ([]model.NotificationEmailTask, error) {
	return nil, nil
}

// fakeRoles 固定返回角色。
type fakeRoles struct{ roles []string }

func (f fakeRoles) Roles(context.Context, uint) ([]string, error) { return f.roles, nil }

func actorPtr(id uint) *uint { return &id }

func readyTask(id, recipient, actor uint) model.NotificationEmailTask {
	return model.NotificationEmailTask{
		Base:            model.Base{ID: id},
		RecipientUserID: recipient,
		ActorUserID:     actorPtr(actor),
		ToEmail:         "owner@x.com",
		Purpose:         "notification",
		Status:          "pending",
		// 相对真实 now 取过去时刻，确保已进入聚合窗口（planner 用 time.Now）。
		AvailableAt: time.Now().Add(-time.Minute),
	}
}

func newPlanner(store *plannerRepoStub, quotaStore *fakeQuotaStore) *notificationservice.EmailPlanner {
	quota := notificationservice.NewQuotaService(quotaStore, cfg())
	return notificationservice.NewEmailPlanner(store, quota, fakeRoles{roles: []string{"normal"}})
}

func looseQuotaStore() *fakeQuotaStore {
	return &fakeQuotaStore{policies: defaultPolicies(), usage: map[string]int{}}
}

// 同一接收人的多条不同来源通知聚合进一封摘要批次。
func TestPlanner_AggregatesSameRecipientIntoOneBatch(t *testing.T) {
	store := &plannerRepoStub{leased: []model.NotificationEmailTask{
		readyTask(1, 5, 2),
		readyTask(2, 5, 3),
		readyTask(3, 5, 4),
	}}
	planner := newPlanner(store, looseQuotaStore())

	created, err := planner.PlanOnce(context.Background(), "worker-1", 50)

	require.NoError(t, err)
	assert.Equal(t, 1, created)
	require.Len(t, store.batches, 1)
	assert.ElementsMatch(t, []uint{1, 2, 3}, store.batches[0])
	assert.Equal(t, 3, store.itemCounts[0])
}

// 不同接收人拆分到不同批次。
func TestPlanner_DifferentRecipientsSplitBatches(t *testing.T) {
	store := &plannerRepoStub{leased: []model.NotificationEmailTask{
		readyTask(1, 5, 2),
		readyTask(2, 6, 2),
	}}
	planner := newPlanner(store, looseQuotaStore())

	created, err := planner.PlanOnce(context.Background(), "worker-1", 50)

	require.NoError(t, err)
	assert.Equal(t, 2, created)
	assert.Len(t, store.batches, 2)
}

// 未到聚合窗口的任务保持 pending（释放租约），不进批次。
func TestPlanner_OutsideWindowStaysPending(t *testing.T) {
	notReady := readyTask(1, 5, 2)
	notReady.AvailableAt = time.Now().Add(10 * time.Minute) // 窗口未到
	store := &plannerRepoStub{leased: []model.NotificationEmailTask{notReady}}
	planner := newPlanner(store, looseQuotaStore())

	created, err := planner.PlanOnce(context.Background(), "worker-1", 50)

	require.NoError(t, err)
	assert.Equal(t, 0, created)
	assert.Empty(t, store.batches)
	assert.Contains(t, store.released, uint(1))
}

// 操作人超限的任务被延后。
func TestPlanner_ActorOverLimitDeferred(t *testing.T) {
	quotaStore := looseQuotaStore()
	quotaStore.roleQuotas = []model.EmailRoleQuotaPolicy{{Role: "normal", ScopeType: "actor", DailyLimit: 30, Enabled: true}}
	quotaStore.usage[usageKey("actor", 2, "*", "day")] = 30 // actor 2 已达上限
	store := &plannerRepoStub{leased: []model.NotificationEmailTask{readyTask(1, 5, 2)}}
	planner := newPlanner(store, quotaStore)

	created, err := planner.PlanOnce(context.Background(), "worker-1", 50)

	require.NoError(t, err)
	assert.Equal(t, 0, created)
	assert.Contains(t, store.deferred, uint(1))
}

// 接收人超限的任务被延后。
func TestPlanner_RecipientOverLimitDeferred(t *testing.T) {
	quotaStore := looseQuotaStore()
	quotaStore.roleQuotas = []model.EmailRoleQuotaPolicy{{Role: "normal", ScopeType: "recipient", DailyLimit: 5, Enabled: true}}
	quotaStore.usage[usageKey("recipient", 5, "*", "day")] = 5
	store := &plannerRepoStub{leased: []model.NotificationEmailTask{readyTask(1, 5, 2)}}
	planner := newPlanner(store, quotaStore)

	created, err := planner.PlanOnce(context.Background(), "worker-1", 50)

	require.NoError(t, err)
	assert.Equal(t, 0, created)
	assert.Contains(t, store.deferred, uint(1))
}

// 批次条数被截断，超出部分保持 pending。
func TestPlanner_BatchItemCountCapped(t *testing.T) {
	leased := make([]model.NotificationEmailTask, 0, 12)
	for i := uint(1); i <= 12; i++ {
		leased = append(leased, readyTask(i, 5, 2))
	}
	store := &plannerRepoStub{leased: leased}
	planner := newPlanner(store, looseQuotaStore())

	created, err := planner.PlanOnce(context.Background(), "worker-1", 50)

	require.NoError(t, err)
	assert.Equal(t, 1, created)
	require.Len(t, store.batches, 1)
	// 默认上限 10，批次只含 10 条，剩余 2 条释放。
	assert.Len(t, store.batches[0], 10)
	assert.Len(t, store.released, 2)
}
