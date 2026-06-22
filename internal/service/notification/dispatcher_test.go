package notification_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// dispatchRepo 是 dispatcher 用的内存假仓储，按唯一键去重以验证幂等。
type dispatchRepo struct {
	pending []model.NotificationEvent

	inboxKeys map[string]struct{} // recipient:event
	taskKeys  map[string]struct{} // idempotency key

	inboxErr error

	doneIDs    []uint
	retryIDs   []uint
	retryTimes map[uint]time.Time
}

func newDispatchRepo(events ...model.NotificationEvent) *dispatchRepo {
	return &dispatchRepo{
		pending:    events,
		inboxKeys:  map[string]struct{}{},
		taskKeys:   map[string]struct{}{},
		retryTimes: map[uint]time.Time{},
	}
}

func (r *dispatchRepo) LeasePendingEvents(context.Context, string, int, int) ([]model.NotificationEvent, error) {
	return r.pending, nil
}
func (r *dispatchRepo) CreateEvent(context.Context, *model.NotificationEvent) error { return nil }
func (r *dispatchRepo) MarkEventDone(_ context.Context, id uint) error {
	r.doneIDs = append(r.doneIDs, id)
	return nil
}
func (r *dispatchRepo) MarkEventRetry(_ context.Context, id uint, next time.Time, _ string) error {
	r.retryIDs = append(r.retryIDs, id)
	r.retryTimes[id] = next
	return nil
}

func (r *dispatchRepo) CreateInbox(_ context.Context, inbox *model.NotificationInbox) (bool, error) {
	if r.inboxErr != nil {
		return false, r.inboxErr
	}
	key := keyOf(inbox.RecipientUserID, inbox.EventID)
	if _, ok := r.inboxKeys[key]; ok {
		return false, nil
	}
	r.inboxKeys[key] = struct{}{}
	return true, nil
}
func (r *dispatchRepo) ListInbox(context.Context, uint, bool, int, int) (*notificationrepo.InboxPage, error) {
	return nil, nil
}
func (r *dispatchRepo) CountUnread(context.Context, uint) (int64, error)         { return 0, nil }
func (r *dispatchRepo) MarkInboxRead(context.Context, uint, uint) (int64, error) { return 0, nil }
func (r *dispatchRepo) MarkAllInboxRead(context.Context, uint, []uint) (int64, error) {
	return 0, nil
}
func (r *dispatchRepo) DeleteInbox(context.Context, uint, uint) (int64, error) { return 0, nil }

func (r *dispatchRepo) CreateEmailTask(_ context.Context, task *model.NotificationEmailTask) (bool, error) {
	if _, ok := r.taskKeys[task.IdempotencyKey]; ok {
		return false, nil
	}
	r.taskKeys[task.IdempotencyKey] = struct{}{}
	return true, nil
}
func (r *dispatchRepo) LeaseEmailTasks(context.Context, string, int, int) ([]model.NotificationEmailTask, error) {
	return nil, nil
}
func (r *dispatchRepo) CreateEmailBatchWithItems(context.Context, *model.NotificationEmailBatch, []uint) error {
	return nil
}
func (r *dispatchRepo) LeaseEmailBatches(context.Context, string, int, int) ([]model.NotificationEmailBatch, error) {
	return nil, nil
}
func (r *dispatchRepo) MarkBatchSent(context.Context, uint, string) error             { return nil }
func (r *dispatchRepo) MarkBatchRetry(context.Context, uint, time.Time, string) error { return nil }
func (r *dispatchRepo) GetPreference(context.Context, uint, string) (*model.NotificationPreference, error) {
	return nil, nil
}
func (r *dispatchRepo) GetQuotaPolicies(context.Context) ([]model.EmailQuotaPolicy, error) {
	return nil, nil
}
func (r *dispatchRepo) GetRoleQuotaPolicies(context.Context) ([]model.EmailRoleQuotaPolicy, error) {
	return nil, nil
}
func (r *dispatchRepo) ReserveQuota(context.Context, notificationrepo.QuotaUsageKey, int) (bool, error) {
	return true, nil
}

func keyOf(recipient, event uint) string {
	return string(rune(recipient)) + ":" + string(rune(event))
}

// 固定接收人解析器与偏好/目录假实现。
type fixedRecipients struct{ ids []uint }

func (f fixedRecipients) Resolve(context.Context, model.NotificationEvent) ([]uint, error) {
	return f.ids, nil
}

type fixedPreference struct {
	pref notificationservice.Preference
}

func (f fixedPreference) Resolve(context.Context, uint, string) (notificationservice.Preference, error) {
	return f.pref, nil
}

type fixedDirectory struct {
	email      string
	canReceive bool
}

func (f fixedDirectory) MailProfile(context.Context, uint) (string, bool, error) {
	return f.email, f.canReceive, nil
}

func commentEvent() model.NotificationEvent {
	actor := uint(2)
	return model.NotificationEvent{
		Base:        model.Base{ID: 10},
		Type:        notificationservice.EventTypeCommentCreated,
		ActorUserID: &actor,
		RootType:    "moment",
		RootID:      12,
	}
}

func bothEnabled() fixedPreference {
	return fixedPreference{pref: notificationservice.Preference{InAppEnabled: true, EmailEnabled: true}}
}

// 评论事件为根对象作者创建一条收件箱。
func TestDispatcher_CommentCreatesInboxForOwner(t *testing.T) {
	repo := newDispatchRepo(commentEvent())
	d := notificationservice.NewDispatcher(repo, fixedRecipients{ids: []uint{5}}, bothEnabled(), fixedDirectory{})

	n, err := d.DispatchOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Len(t, repo.inboxKeys, 1)
	assert.Contains(t, repo.inboxKeys, keyOf(5, 10))
	assert.Equal(t, []uint{10}, repo.doneIDs)
}

// 操作人就是接收人时不通知自己。
func TestDispatcher_SelfActionDoesNotNotifySelf(t *testing.T) {
	repo := newDispatchRepo(commentEvent())
	// 接收人即 actor(2)。
	d := notificationservice.NewDispatcher(repo, fixedRecipients{ids: []uint{2}}, bothEnabled(), fixedDirectory{email: "a@x.com", canReceive: true})

	n, err := d.DispatchOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Empty(t, repo.inboxKeys)
	assert.Empty(t, repo.taskKeys)
}

// 关闭站内偏好时跳过收件箱。
func TestDispatcher_DisabledInAppSkipsInbox(t *testing.T) {
	repo := newDispatchRepo(commentEvent())
	pref := fixedPreference{pref: notificationservice.Preference{InAppEnabled: false, EmailEnabled: false}}
	d := notificationservice.NewDispatcher(repo, fixedRecipients{ids: []uint{5}}, pref, fixedDirectory{})

	_, err := d.DispatchOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Empty(t, repo.inboxKeys)
}

// 开启邮件偏好且可收件时创建邮件任务。
func TestDispatcher_EnabledEmailCreatesTask(t *testing.T) {
	repo := newDispatchRepo(commentEvent())
	d := notificationservice.NewDispatcher(repo, fixedRecipients{ids: []uint{5}}, bothEnabled(), fixedDirectory{email: "owner@x.com", canReceive: true})

	_, err := d.DispatchOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Len(t, repo.taskKeys, 1)
	assert.Contains(t, repo.taskKeys, "event:10:user:5")
}

// 总开关 receive_mail 关闭时跳过邮件任务。
func TestDispatcher_DisabledReceiveMailSkipsTask(t *testing.T) {
	repo := newDispatchRepo(commentEvent())
	d := notificationservice.NewDispatcher(repo, fixedRecipients{ids: []uint{5}}, bothEnabled(), fixedDirectory{email: "owner@x.com", canReceive: false})

	_, err := d.DispatchOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Empty(t, repo.taskKeys)
	// 站内仍然投递。
	assert.Len(t, repo.inboxKeys, 1)
}

// 重复分发不会重复写收件箱或邮件任务。
func TestDispatcher_DuplicateDispatchIsIdempotent(t *testing.T) {
	repo := newDispatchRepo(commentEvent())
	d := notificationservice.NewDispatcher(repo, fixedRecipients{ids: []uint{5}}, bothEnabled(), fixedDirectory{email: "owner@x.com", canReceive: true})

	_, err := d.DispatchOnce(context.Background(), "worker-1", 10)
	require.NoError(t, err)
	_, err = d.DispatchOnce(context.Background(), "worker-1", 10)
	require.NoError(t, err)

	assert.Len(t, repo.inboxKeys, 1)
	assert.Len(t, repo.taskKeys, 1)
}

// 点赞类事件不在邮件白名单内：只写收件箱，不创建邮件任务。
func TestDispatcher_LikeEventStaysInAppOnly(t *testing.T) {
	actor := uint(2)
	likeEvent := model.NotificationEvent{
		Base:        model.Base{ID: 20},
		Type:        notificationservice.EventTypeMomentLiked,
		ActorUserID: &actor,
		RootType:    "moment",
		RootID:      12,
	}
	repo := newDispatchRepo(likeEvent)
	d := notificationservice.NewDispatcher(repo, fixedRecipients{ids: []uint{5}}, bothEnabled(), fixedDirectory{email: "owner@x.com", canReceive: true})

	_, err := d.DispatchOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Len(t, repo.inboxKeys, 1)
	assert.Empty(t, repo.taskKeys)
}

// 仓储出错时事件被回退重试并设置未来的下次处理时间。
func TestDispatcher_RetriesEventOnRepoError(t *testing.T) {
	repo := newDispatchRepo(commentEvent())
	repo.inboxErr = errors.New("db down")
	d := notificationservice.NewDispatcher(repo, fixedRecipients{ids: []uint{5}}, bothEnabled(), fixedDirectory{})

	n, err := d.DispatchOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, []uint{10}, repo.retryIDs)
	assert.Empty(t, repo.doneIDs)
	assert.True(t, repo.retryTimes[10].After(time.Now()))
}
