package moderationemail

import (
	"context"
	"time"
)

// PlanOnce 清理陈旧任务，并在聚合窗口和冷却时间均到期后创建一个批次。
// workerID 预留给统一 worker 接口；并发正确性由 CreateBatch 的事务锁与状态复查保证。
func (p *Planner) PlanOnce(ctx context.Context, workerID string, limit int) (int, error) {
	now := p.now()
	if p.recipientRetryWaiting(now) {
		return 0, nil
	}

	// 先清理已不再待审的任务，避免陈旧任务参与后续规划。
	if err := p.repo.SkipStaleTasks(ctx, limit, now); err != nil {
		return 0, err
	}

	// 开放批次必须先发送或恢复，当前轮次不再创建第二个批次。
	open, err := p.repo.HasOpenBatch(ctx)
	if err != nil || open {
		return 0, err
	}

	// 无有效待处理任务时结束本轮规划。
	oldest, err := p.repo.OldestPendingTask(ctx)
	if err != nil || oldest == nil {
		return 0, err
	}

	// 聚合窗口与最近发送冷却时间必须同时满足。
	lastSent, err := p.repo.LastSuccessfulSend(ctx)
	if err != nil {
		return 0, err
	}
	if now.Before(nextDue(oldest.AvailableAt, lastSent, p.cfg.MinInterval)) {
		return 0, nil
	}

	// 创建批次前加载最新的合格管理员邮箱快照。
	recipient, err := p.directory.LoadAdminRecipient(ctx, p.cfg.RecipientUserID)
	if err != nil {
		p.deferRecipientRetry(now)
		return 0, err
	}

	// 仓储事务会重新锁定并复查当前待审任务，预检查仅用于减少无效事务。
	return p.repo.CreateBatch(ctx, recipient, limit, now)
}

func (p *Planner) recipientRetryWaiting(now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return now.Before(p.retryAt)
}

func (p *Planner) deferRecipientRetry(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retryAt = now.Add(p.recipientRetryInterval())
}

func (p *Planner) recipientRetryInterval() time.Duration {
	if p.cfg.RecipientRetryInterval > 0 {
		return p.cfg.RecipientRetryInterval
	}
	if p.cfg.MinInterval > 0 {
		return p.cfg.MinInterval
	}
	return 30 * time.Minute
}

func nextDue(availableAt time.Time, lastSent *time.Time, minInterval time.Duration) time.Time {
	if lastSent == nil {
		return availableAt
	}
	return maxTime(availableAt, lastSent.Add(minInterval))
}

func maxTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}
