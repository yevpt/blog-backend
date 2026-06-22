package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
)

// 规划默认参数。
const (
	defaultMaxBatchItems    = 10  // 单封摘要邮件最多聚合的通知条数
	defaultPlannerLeaseSecs = 300 // planner 领取任务的租约秒数
)

// RoleResolver 解析用户角色，供额度评估按角色限额。
type RoleResolver interface {
	Roles(ctx context.Context, userID uint) ([]string, error)
}

// plannerRepo 是 planner 依赖的仓储能力子集。
type plannerRepo interface {
	notificationrepo.EmailTaskRepository
	notificationrepo.EmailBatchRepository
}

// EmailPlanner 邮件聚合规划器：领取待处理任务，按接收人聚合为邮件批次。
// 跨对象聚合——同一接收人同窗口内的不同来源通知合成一封摘要；额度不足的任务延后。
type EmailPlanner struct {
	repo          plannerRepo
	quota         *QuotaService
	roles         RoleResolver
	maxBatchItems int
	leaseSeconds  int
	now           func() time.Time
}

// NewEmailPlanner 创建邮件聚合规划器。
func NewEmailPlanner(repo plannerRepo, quota *QuotaService, roles RoleResolver) *EmailPlanner {
	return &EmailPlanner{
		repo:          repo,
		quota:         quota,
		roles:         roles,
		maxBatchItems: defaultMaxBatchItems,
		leaseSeconds:  defaultPlannerLeaseSecs,
		now:           time.Now,
	}
}

// planGroupKey 聚合分组键：同一接收人、同一邮箱、同一用途合成一封邮件。
type planGroupKey struct {
	recipientUserID uint
	toEmail         string
	purpose         string
}

// PlanOnce 领取一批任务并聚合为邮件批次，返回新建的批次数。
func (p *EmailPlanner) PlanOnce(ctx context.Context, workerID string, limit int) (int, error) {
	tasks, err := p.repo.LeaseEmailTasks(ctx, workerID, p.leaseSeconds, limit)
	if err != nil {
		return 0, err
	}
	now := p.now()

	// 未到聚合窗口的任务释放回 pending，等待下次累积；其余按接收人分组。
	groups := make(map[planGroupKey][]model.NotificationEmailTask)
	var notReady []uint
	for _, task := range tasks {
		if task.AvailableAt.After(now) {
			notReady = append(notReady, task.ID)
			continue
		}
		key := planGroupKey{recipientUserID: task.RecipientUserID, toEmail: task.ToEmail, purpose: task.Purpose}
		groups[key] = append(groups[key], task)
	}
	if err := p.repo.ReleaseEmailTasks(ctx, notReady); err != nil {
		return 0, err
	}

	created := 0
	for key, groupTasks := range groups {
		ok, err := p.planGroup(ctx, key, groupTasks, now)
		if err != nil {
			return created, err
		}
		if ok {
			created++
		}
	}
	return created, nil
}

// planGroup 处理单个接收人分组：额度评估、容量截断、生成批次。
func (p *EmailPlanner) planGroup(ctx context.Context, key planGroupKey, tasks []model.NotificationEmailTask, now time.Time) (bool, error) {
	recipientRoles, err := p.roles.Roles(ctx, key.recipientUserID)
	if err != nil {
		return false, err
	}

	// 逐任务做额度评估：通过的进批，超限的延后。
	var include []model.NotificationEmailTask
	var deferIDs []uint
	var deferUntil time.Time
	for _, task := range tasks {
		decision, err := p.evaluateTask(ctx, key, task, recipientRoles, now)
		if err != nil {
			return false, err
		}
		if !decision.Allowed {
			deferIDs = append(deferIDs, task.ID)
			if decision.DeferUntil.After(deferUntil) {
				deferUntil = decision.DeferUntil
			}
			continue
		}
		include = append(include, task)
	}

	// 超限任务统一延后。
	if len(deferIDs) > 0 {
		if err := p.repo.DeferEmailTasks(ctx, deferIDs, deferUntil); err != nil {
			return false, err
		}
	}
	if len(include) == 0 {
		return false, nil
	}

	// 容量截断：超出上限的部分释放回 pending，等下次再聚合。
	if len(include) > p.maxBatchItems {
		overflow := include[p.maxBatchItems:]
		if err := p.repo.ReleaseEmailTasks(ctx, taskIDs(overflow)); err != nil {
			return false, err
		}
		include = include[:p.maxBatchItems]
	}

	// 生成批次与连接行，并把任务标记为 batched。
	batch := &model.NotificationEmailBatch{
		RecipientUserID: key.recipientUserID,
		ToEmail:         key.toEmail,
		Purpose:         key.purpose,
		Subject:         digestSubject(len(include)),
		Status:          notificationrepo.EmailBatchStatusPending,
		ItemCount:       len(include),
		ScheduledAt:     now,
	}
	if err := p.repo.CreateEmailBatchWithItems(ctx, batch, taskIDs(include)); err != nil {
		return false, err
	}
	return true, nil
}

// evaluateTask 评估单条任务的额度；系统通知等无操作人的任务跳过 actor 维度。
func (p *EmailPlanner) evaluateTask(ctx context.Context, key planGroupKey, task model.NotificationEmailTask, recipientRoles []string, now time.Time) (QuotaDecision, error) {
	var actorUserID uint
	var actorRoles []string
	if task.ActorUserID != nil {
		actorUserID = *task.ActorUserID
		roles, err := p.roles.Roles(ctx, actorUserID)
		if err != nil {
			return QuotaDecision{}, err
		}
		actorRoles = roles
	}
	return p.quota.Evaluate(ctx, QuotaInput{
		Purpose:         key.purpose,
		ActorUserID:     actorUserID,
		RecipientUserID: key.recipientUserID,
		ActorRoles:      actorRoles,
		RecipientRoles:  recipientRoles,
		Now:             now,
	})
}

// taskIDs 提取任务 ID 列表。
func taskIDs(tasks []model.NotificationEmailTask) []uint {
	ids := make([]uint, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

// digestSubject 生成摘要邮件标题。
func digestSubject(count int) string {
	return fmt.Sprintf("你有 %d 条新通知", count)
}
