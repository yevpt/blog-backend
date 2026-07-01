package moderationemail_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
	moderationemailrepo "github.com/vpt/blog-backend/internal/repository/moderationemail"
	moderationemailservice "github.com/vpt/blog-backend/internal/service/moderationemail"
)

func TestSenderSuccessUsesStableMessageIDAndMarksSent(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	repo := &senderRepoStub{
		batches: []model.ModerationReviewEmailBatch{{
			ID: 9, ToEmail: "owner@example.com", ItemCount: 1, Attempts: 0,
		}},
		tasks: map[uint64][]moderationemailrepo.PendingTask{
			9: {reviewTask(1, model.ModerationContentMoment, "待审核正文")},
		},
	}
	mailer := &reviewMailerStub{beforeSend: func() {
		assert.Equal(t, []sentCall{{batchID: 9, messageID: "moderation-review-batch-9", at: now}}, repo.persisted)
	}}
	sender := moderationemailservice.NewSender(repo, mailer, "https://blog.example.com", func() time.Time { return now })

	sent, err := sender.SendOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Equal(t, 1, sent)
	require.Len(t, mailer.calls, 1)
	assert.Equal(t, "owner@example.com", mailer.calls[0].to)
	assert.Equal(t, "待审核内容提醒（1 条）", mailer.calls[0].subject)
	assert.Equal(t, "moderation-review-batch-9", mailer.calls[0].messageID)
	assert.Equal(t, []sentCall{{batchID: 9, messageID: "moderation-review-batch-9", at: now}}, repo.persisted)
	assert.Equal(t, []sentCall{{batchID: 9, messageID: "moderation-review-batch-9", at: now}}, repo.sent)
	assert.Empty(t, repo.retries)
}

func TestSenderSMTPFailureRetriesWithoutSentTimestampAndReusesPersistedMessageID(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	persisted := "moderation-review-batch-9"
	repo := &senderRepoStub{
		batches: []model.ModerationReviewEmailBatch{{
			ID: 9, ToEmail: "owner@example.com", ItemCount: 1, Attempts: 2, MessageID: &persisted,
		}},
		tasks: map[uint64][]moderationemailrepo.PendingTask{
			9: {reviewTask(1, model.ModerationContentMoment, "待审核正文")},
		},
	}
	mailer := &reviewMailerStub{err: errors.New("smtp down")}
	sender := moderationemailservice.NewSender(repo, mailer, "https://blog.example.com", func() time.Time { return now })

	sent, err := sender.SendOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Zero(t, sent)
	require.Len(t, mailer.calls, 1)
	assert.Equal(t, "moderation-review-batch-9", mailer.calls[0].messageID)
	require.Len(t, repo.retries, 1)
	assert.Equal(t, retryCall{batchID: 9, messageID: "moderation-review-batch-9", lastErr: "smtp down"}, repo.retries[0].withoutTime())
	assert.Greater(t, repo.retries[0].nextAttemptAt.Sub(now), time.Duration(0))
	assert.LessOrEqual(t, repo.retries[0].nextAttemptAt.Sub(now), 30*time.Minute)
	assert.Empty(t, repo.sent)
}

func TestSenderMarkSentFailureReleasesLeaseForRetryWithoutSecondSMTPAttempt(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	repo := &senderRepoStub{
		batches: []model.ModerationReviewEmailBatch{{
			ID: 9, ToEmail: "owner@example.com", ItemCount: 1, Attempts: 1,
		}},
		tasks: map[uint64][]moderationemailrepo.PendingTask{
			9: {reviewTask(1, model.ModerationContentMoment, "待审核正文")},
		},
		markSentErr: errors.New("db unavailable"),
	}
	mailer := &reviewMailerStub{}
	sender := moderationemailservice.NewSender(repo, mailer, "", func() time.Time { return now })

	sent, err := sender.SendOnce(context.Background(), "worker-1", 10)

	require.NoError(t, err)
	assert.Zero(t, sent)
	assert.Len(t, mailer.calls, 1)
	assert.Empty(t, repo.sent)
	require.Len(t, repo.retries, 1)
	assert.Equal(t, "moderation-review-batch-9", repo.retries[0].messageID)
	assert.Equal(t, "db unavailable", repo.retries[0].lastErr)
}

type senderRepoStub struct {
	batches     []model.ModerationReviewEmailBatch
	tasks       map[uint64][]moderationemailrepo.PendingTask
	persisted   []sentCall
	sent        []sentCall
	retries     []retryCall
	leaseArgs   leaseCall
	markSentErr error
}

type leaseCall struct {
	workerID string
	limit    int
}

type sentCall struct {
	batchID   uint64
	messageID string
	at        time.Time
}

type retryCall struct {
	batchID       uint64
	messageID     string
	nextAttemptAt time.Time
	lastErr       string
	at            time.Time
}

func (c retryCall) withoutTime() retryCall {
	c.nextAttemptAt = time.Time{}
	c.at = time.Time{}
	return c
}

func (s *senderRepoStub) LeaseBatches(_ context.Context, workerID string, _ time.Duration, limit int, _ time.Time) ([]model.ModerationReviewEmailBatch, error) {
	s.leaseArgs = leaseCall{workerID: workerID, limit: limit}
	return s.batches, nil
}

func (s *senderRepoStub) LoadBatchTasks(_ context.Context, batchID uint64, limit int) ([]moderationemailrepo.PendingTask, error) {
	if limit != 50 {
		return nil, errors.New("unexpected load limit")
	}
	return s.tasks[batchID], nil
}

func (s *senderRepoStub) PersistBatchMessageID(_ context.Context, batchID uint64, messageID string, now time.Time) error {
	s.persisted = append(s.persisted, sentCall{batchID: batchID, messageID: messageID, at: now})
	return nil
}

func (s *senderRepoStub) MarkBatchSent(_ context.Context, batchID uint64, messageID string, now time.Time) error {
	if s.markSentErr != nil {
		return s.markSentErr
	}
	s.sent = append(s.sent, sentCall{batchID: batchID, messageID: messageID, at: now})
	return nil
}

func (s *senderRepoStub) MarkBatchRetry(_ context.Context, batchID uint64, messageID string, nextAttemptAt time.Time, lastErr string, now time.Time) error {
	s.retries = append(s.retries, retryCall{
		batchID:       batchID,
		messageID:     messageID,
		nextAttemptAt: nextAttemptAt,
		lastErr:       lastErr,
		at:            now,
	})
	return nil
}

type reviewMailerStub struct {
	calls      []mailCall
	err        error
	beforeSend func()
}

type mailCall struct {
	to        string
	subject   string
	body      string
	messageID string
}

func (m *reviewMailerStub) SendVerificationCode(string, string) error { return nil }

func (m *reviewMailerStub) SendHTML(to string, subject string, body string, messageID string) error {
	if m.beforeSend != nil {
		m.beforeSend()
	}
	m.calls = append(m.calls, mailCall{to: to, subject: subject, body: body, messageID: messageID})
	return m.err
}
