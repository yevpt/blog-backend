package moderation

import (
	"context"
	"time"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	"go.uber.org/zap"
)

const (
	publishRecoveryInterval = time.Minute
	publishRecoveryBatch    = 50
)

// PublishRecoveryWorker 重试已提交审核事务但尚未完成的碎语图片正式化。
type PublishRecoveryWorker struct {
	repo      moderationrepo.PublishRecoveryRepository
	publisher moderationmedia.Publisher
	logger    *zap.Logger
}

// NewPublishRecoveryWorker 通过构造注入创建正式化补偿 worker。
func NewPublishRecoveryWorker(
	repo moderationrepo.PublishRecoveryRepository,
	publisher moderationmedia.Publisher,
	logger *zap.Logger,
) *PublishRecoveryWorker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PublishRecoveryWorker{repo: repo, publisher: publisher, logger: logger}
}

// Run 启动即补偿一次，之后按固定短间隔重试。
func (w *PublishRecoveryWorker) Run(ctx context.Context) {
	if w == nil || w.repo == nil || w.publisher == nil {
		return
	}
	ticker := time.NewTicker(publishRecoveryInterval)
	defer ticker.Stop()
	for {
		w.RecoverOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RecoverOnce 有界重试一批图片正式化任务，单条失败不阻塞其余任务。
func (w *PublishRecoveryWorker) RecoverOnce(ctx context.Context) int {
	rows, err := w.repo.ListPublishRecoveryCandidates(ctx, publishRecoveryBatch)
	if err != nil {
		w.logger.Warn("查询碎语图片正式化补偿任务失败", zap.Error(err))
		return 0
	}
	recovered := 0
	for _, row := range rows {
		current, loadErr := w.repo.LoadRevisionImages(ctx, row.RevisionID)
		if loadErr != nil {
			w.logger.Warn("加载待补偿碎语图片失败", zap.Uint64("item_id", row.ItemID), zap.Error(loadErr))
			continue
		}
		var previous []moderationrepo.RevisionImageRecord
		if row.PreviousRevisionID != nil {
			previous, loadErr = w.repo.LoadRevisionImages(ctx, *row.PreviousRevisionID)
			if loadErr != nil {
				w.logger.Warn("加载上一碎语审核版本图片失败", zap.Uint64("item_id", row.ItemID), zap.Error(loadErr))
				continue
			}
		}
		_, publishErr := w.publisher.Publish(ctx, moderationmedia.PublishCommand{
			ItemID: row.ItemID, RevisionID: row.RevisionID, UserID: row.AuthorID,
			MomentID: row.MomentID, Current: current, Previous: previous,
		})
		if publishErr != nil {
			w.logger.Warn("补偿碎语图片正式化失败，等待下次重试",
				zap.Uint64("item_id", row.ItemID), zap.Error(publishErr))
			continue
		}
		recovered++
	}
	return recovered
}
