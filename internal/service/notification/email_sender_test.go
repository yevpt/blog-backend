package notification_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// senderRepoStub 驱动 sender 并记录其状态写入与日志。
type senderRepoStub struct {
	batches    []model.NotificationEmailBatch
	batchTasks map[uint][]model.NotificationEmailTask
	events     map[uint]model.NotificationEvent

	sentBatch  []uint
	retryBatch []uint
	logs       []model.EmailSendLog
}

func (s *senderRepoStub) LeaseEmailBatches(context.Context, string, int, int) ([]model.NotificationEmailBatch, error) {
	return s.batches, nil
}
func (s *senderRepoStub) CreateEmailBatchWithItems(context.Context, *model.NotificationEmailBatch, []uint) error {
	return nil
}
func (s *senderRepoStub) MarkBatchSent(_ context.Context, id uint, _ string) error {
	s.sentBatch = append(s.sentBatch, id)
	return nil
}
func (s *senderRepoStub) MarkBatchRetry(_ context.Context, id uint, _ time.Time, _ string) error {
	s.retryBatch = append(s.retryBatch, id)
	return nil
}
func (s *senderRepoStub) ListBatchTasks(_ context.Context, batchID uint) ([]model.NotificationEmailTask, error) {
	return s.batchTasks[batchID], nil
}
func (s *senderRepoStub) CreateSendLog(_ context.Context, log *model.EmailSendLog) error {
	s.logs = append(s.logs, *log)
	return nil
}
func (s *senderRepoStub) GetEventsByIDs(_ context.Context, ids []uint) (map[uint]model.NotificationEvent, error) {
	out := map[uint]model.NotificationEvent{}
	for _, id := range ids {
		if e, ok := s.events[id]; ok {
			out[id] = e
		}
	}
	return out, nil
}

// fakeMailer 记录 SendHTML 调用，可配置返回错误。
type fakeMailer struct {
	calls    int
	lastBody string
	err      error
}

func (m *fakeMailer) SendVerificationCode(string, string) error { return nil }
func (m *fakeMailer) SendHTML(_ string, _ string, body string, _ string) error {
	m.calls++
	m.lastBody = body
	return m.err
}

func oneBatch() model.NotificationEmailBatch {
	return model.NotificationEmailBatch{
		Base:            model.Base{ID: 1},
		RecipientUserID: 5,
		ToEmail:         "owner@x.com",
		Purpose:         "notification",
		Subject:         "你有 2 条新通知",
		Status:          "pending",
		ItemCount:       2,
	}
}

func newSender(repo *senderRepoStub, quotaStore *fakeQuotaStore, mailer *fakeMailer) *notificationservice.EmailSender {
	quota := notificationservice.NewQuotaService(quotaStore, cfg())
	return notificationservice.NewEmailSender(repo, quota, fakeRoles{roles: []string{"normal"}}, mailer, "test")
}

// 发送前先校验额度；额度足够时成功发送并标记 sent，落成功日志。
func TestSender_SuccessfulSendMarksSent(t *testing.T) {
	repo := &senderRepoStub{
		batches:    []model.NotificationEmailBatch{oneBatch()},
		batchTasks: map[uint][]model.NotificationEmailTask{1: {{Base: model.Base{ID: 10}, EventID: 100}}},
		events:     map[uint]model.NotificationEvent{100: {Base: model.Base{ID: 100}, Type: "comment_created", ContentExcerpt: "好文章"}},
	}
	mailer := &fakeMailer{}
	sender := newSender(repo, looseQuotaStore(), mailer)

	n, err := sender.SendOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, mailer.calls)
	assert.Equal(t, []uint{1}, repo.sentBatch)
	require.Len(t, repo.logs, 1)
	assert.Equal(t, "success", repo.logs[0].Status)
}

// 超过全站每分钟上限时延后批次且不调用 SMTP。
func TestSender_MinuteLimitDefersWithoutSMTP(t *testing.T) {
	quotaStore := looseQuotaStore()
	quotaStore.usage[usageKey("site", 0, "*", "minute")] = 5 // 已达每分钟上限
	repo := &senderRepoStub{batches: []model.NotificationEmailBatch{oneBatch()}}
	mailer := &fakeMailer{}
	sender := newSender(repo, quotaStore, mailer)

	n, err := sender.SendOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, mailer.calls)
	assert.Equal(t, []uint{1}, repo.retryBatch)
	assert.Empty(t, repo.sentBatch)
}

// SMTP 失败时记录失败日志并安排重试。
func TestSender_SMTPFailureRecordsLogAndRetries(t *testing.T) {
	repo := &senderRepoStub{
		batches:    []model.NotificationEmailBatch{oneBatch()},
		batchTasks: map[uint][]model.NotificationEmailTask{1: {{Base: model.Base{ID: 10}, EventID: 100}}},
		events:     map[uint]model.NotificationEvent{100: {Base: model.Base{ID: 100}, Type: "comment_created"}},
	}
	mailer := &fakeMailer{err: errors.New("smtp down")}
	sender := newSender(repo, looseQuotaStore(), mailer)

	n, err := sender.SendOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, []uint{1}, repo.retryBatch)
	require.Len(t, repo.logs, 1)
	assert.Equal(t, "failed", repo.logs[0].Status)
}

// 渲染的摘要包含多种事件类型的内容。
func TestSender_RenderedDigestIncludesMultipleEventTypes(t *testing.T) {
	repo := &senderRepoStub{
		batches: []model.NotificationEmailBatch{oneBatch()},
		batchTasks: map[uint][]model.NotificationEmailTask{1: {
			{Base: model.Base{ID: 10}, EventID: 100},
			{Base: model.Base{ID: 11}, EventID: 101},
		}},
		events: map[uint]model.NotificationEvent{
			100: {Base: model.Base{ID: 100}, Type: "comment_created", ContentExcerpt: "评论内容"},
			101: {Base: model.Base{ID: 101}, Type: "reply_created", ContentExcerpt: "回复内容"},
		},
	}
	mailer := &fakeMailer{}
	sender := newSender(repo, looseQuotaStore(), mailer)

	_, err := sender.SendOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.True(t, strings.Contains(mailer.lastBody, "评论"))
	assert.True(t, strings.Contains(mailer.lastBody, "回复"))
	assert.True(t, strings.Contains(mailer.lastBody, "评论内容"))
	assert.True(t, strings.Contains(mailer.lastBody, "回复内容"))
}
