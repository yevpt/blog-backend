package bootstrap_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/bootstrap"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestStartModerationReviewEmailWorkerSkipsWhenModerationDisabled(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	cfg := moderationReviewEmailWorkerConfig()
	cfg.Moderation.Enabled = false
	tasks, cancel := newCanceledTaskGroup(zap.New(core))
	defer cancel()

	bootstrap.StartModerationReviewEmailWorker(tasks, cfg, nil, nil, zap.New(core))

	assert.Empty(t, logs.All())
}

func TestStartModerationReviewEmailWorkerSkipsWhenReviewEmailDisabled(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	cfg := moderationReviewEmailWorkerConfig()
	cfg.Moderation.ReviewEmail.Enabled = false
	tasks, cancel := newCanceledTaskGroup(zap.New(core))
	defer cancel()

	bootstrap.StartModerationReviewEmailWorker(tasks, cfg, nil, nil, zap.New(core))

	assert.Empty(t, logs.All())
}

func TestStartModerationReviewEmailWorkerSkipsWhenAsyncEmailWorkerDisabled(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	cfg := moderationReviewEmailWorkerConfig()
	cfg.Email.WorkerEnabled = false
	tasks, cancel := newCanceledTaskGroup(zap.New(core))
	defer cancel()

	bootstrap.StartModerationReviewEmailWorker(tasks, cfg, nil, nil, zap.New(core))

	assert.Empty(t, logs.All())
}

func TestStartModerationReviewEmailWorkerStartsWhenEnabled(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	cfg := moderationReviewEmailWorkerConfig()
	tasks, cancel := newCanceledTaskGroup(zap.New(core))
	defer cancel()

	bootstrap.StartModerationReviewEmailWorker(tasks, cfg, nil, nil, zap.New(core))
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	assert.NoError(t, tasks.Wait(waitCtx))

	assert.Len(t, logs.FilterMessage("审核待处理邮件 worker 启动").All(), 1)
}

func newCanceledTaskGroup(logger *zap.Logger) (*bootstrap.TaskGroup, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return bootstrap.NewTaskGroup(ctx, logger), cancel
}

func moderationReviewEmailWorkerConfig() *config.Config {
	return &config.Config{
		Email: config.EmailConfig{
			WorkerEnabled: true,
			SiteURL:       "https://blog.example.com",
		},
		Moderation: config.ModerationConfig{
			Enabled: true,
			ReviewEmail: config.ModerationReviewEmailConfig{
				Enabled:                  true,
				RecipientUserID:          1,
				AggregationWindowSeconds: 60,
				MinIntervalSeconds:       1800,
				PollIntervalSeconds:      int((15 * time.Second).Seconds()),
			},
		},
	}
}
