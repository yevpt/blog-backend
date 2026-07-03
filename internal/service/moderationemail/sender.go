package moderationemail

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	moderationemailrepo "github.com/vpt/blog-backend/internal/repository/moderationemail"
)

const (
	defaultSenderLeaseDuration = 5 * time.Minute
	baseRetryDelay             = time.Minute
	maxRetryDelay              = 30 * time.Minute
)

// SenderRepository 定义审核邮件发送所需的最小持久化能力。
type SenderRepository interface {
	LeaseBatches(ctx context.Context, workerID string, leaseDuration time.Duration, limit int, now time.Time) ([]model.ModerationReviewEmailBatch, error)
	LoadBatchTasks(ctx context.Context, batchID uint64, limit int) ([]moderationemailrepo.PendingTask, error)
	PersistBatchMessageID(ctx context.Context, batchID uint64, messageID string, now time.Time) error
	MarkBatchSent(ctx context.Context, batchID uint64, messageID string, now time.Time) error
	MarkBatchSkipped(ctx context.Context, batchID uint64, messageID string, lastErr string, now time.Time) error
	MarkBatchRetry(ctx context.Context, batchID uint64, messageID string, nextAttemptAt time.Time, lastErr string, now time.Time) error
}

// MailSender 定义审核摘要邮件发送能力。
type MailSender interface {
	SendHTML(to string, subject string, htmlBody string, messageID string) error
}

// Sender 领取待发送审核摘要批次并执行可重试发送。
type Sender struct {
	repo          SenderRepository
	mailer        MailSender
	siteURL       string
	brandName     string
	adminURL      string
	leaseDuration time.Duration
	now           func() time.Time
}

// NewSender 创建审核摘要邮件发送器。
func NewSender(repo SenderRepository, mailer MailSender, siteURL, brandName, adminURL string, now func() time.Time) *Sender {
	if now == nil {
		now = time.Now
	}
	return &Sender{
		repo:          repo,
		mailer:        mailer,
		siteURL:       siteURL,
		brandName:     brandName,
		adminURL:      adminURL,
		leaseDuration: defaultSenderLeaseDuration,
		now:           now,
	}
}

// SendOnce 领取并发送一轮审核摘要邮件，返回成功发送的批次数。
func (s *Sender) SendOnce(ctx context.Context, workerID string, limit int) (int, error) {
	now := s.now()

	// 先领取可发送批次；领取失败直接返回，避免误报已发送。
	batches, err := s.repo.LeaseBatches(ctx, workerID, s.leaseDuration, limit, now)
	if err != nil {
		return 0, err
	}

	// 逐批发送，单批失败会释放租约并进入退避，不阻断后续批次。
	sent := 0
	for _, batch := range batches {
		ok, err := s.sendBatch(ctx, batch)
		if err != nil {
			return sent, err
		}
		if ok {
			sent++
		}
	}
	return sent, nil
}

func (s *Sender) sendBatch(ctx context.Context, batch model.ModerationReviewEmailBatch) (bool, error) {
	now := s.now()
	messageID := stableMessageID(batch)

	// 首次尝试前先持久化 Message-ID，后续失败重试复用同一个值。
	if batch.MessageID == nil || *batch.MessageID == "" {
		if err := s.repo.PersistBatchMessageID(ctx, batch.ID, messageID, now); err != nil {
			return false, s.retryBatch(ctx, batch, messageID, err, now)
		}
	}

	// 加载最多 50 条展示快照；总数仍使用批次 ItemCount。
	tasks, err := s.repo.LoadBatchTasks(ctx, batch.ID, maxRenderedRows)
	if err != nil {
		return false, s.retryBatch(ctx, batch, messageID, err, now)
	}
	if len(tasks) == 0 {
		return false, s.repo.MarkBatchSkipped(ctx, batch.ID, messageID, "no current pending review email tasks", now)
	}
	renderBatch := batch
	if len(tasks) < maxRenderedRows && renderBatch.ItemCount != len(tasks) {
		renderBatch.ItemCount = len(tasks)
	}
	rendered, err := Render(renderBatch, tasks, s.siteURL, s.brandName, s.adminURL)
	if err != nil {
		return false, s.retryBatch(ctx, batch, messageID, err, now)
	}

	// SMTP 失败只安排重试，不写 sent_at。
	if err := s.mailer.SendHTML(batch.ToEmail, rendered.Subject, rendered.HTML, messageID); err != nil {
		return false, s.retryBatch(ctx, batch, messageID, err, now)
	}

	// 成功后原子标记批次和任务；落库失败也释放租约并重试。
	if err := s.repo.MarkBatchSent(ctx, batch.ID, messageID, now); err != nil {
		return false, s.retryBatch(ctx, batch, messageID, err, now)
	}
	return true, nil
}

func (s *Sender) retryBatch(ctx context.Context, batch model.ModerationReviewEmailBatch, messageID string, cause error, now time.Time) error {
	nextAttemptAt := now.Add(retryDelay(batch.Attempts))
	return s.repo.MarkBatchRetry(ctx, batch.ID, messageID, nextAttemptAt, cause.Error(), now)
}

func stableMessageID(batch model.ModerationReviewEmailBatch) string {
	if batch.MessageID != nil && *batch.MessageID != "" {
		return *batch.MessageID
	}
	return fmt.Sprintf("moderation-review-batch-%d", batch.ID)
}

func retryDelay(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	delay := baseRetryDelay
	for i := 0; i < attempts; i++ {
		delay *= 2
		if delay >= maxRetryDelay {
			return maxRetryDelay
		}
	}
	return delay
}
