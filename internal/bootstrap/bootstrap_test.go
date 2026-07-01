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

	bootstrap.StartModerationReviewEmailWorker(context.Background(), cfg, nil, nil, zap.New(core))

	assert.Empty(t, logs.All())
}

func TestStartModerationReviewEmailWorkerSkipsWhenReviewEmailDisabled(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	cfg := moderationReviewEmailWorkerConfig()
	cfg.Moderation.ReviewEmail.Enabled = false

	bootstrap.StartModerationReviewEmailWorker(context.Background(), cfg, nil, nil, zap.New(core))

	assert.Empty(t, logs.All())
}

func TestStartModerationReviewEmailWorkerSkipsWhenAsyncEmailWorkerDisabled(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	cfg := moderationReviewEmailWorkerConfig()
	cfg.Email.WorkerEnabled = false

	bootstrap.StartModerationReviewEmailWorker(context.Background(), cfg, nil, nil, zap.New(core))

	assert.Empty(t, logs.All())
}

func TestStartModerationReviewEmailWorkerStartsWhenEnabled(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	cfg := moderationReviewEmailWorkerConfig()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	bootstrap.StartModerationReviewEmailWorker(ctx, cfg, nil, nil, zap.New(core))

	assert.Len(t, logs.FilterMessage("审核待处理邮件 worker 启动").All(), 1)
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
