package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
	"github.com/vpt/blog-backend/pkg/email"
)

// 发送默认参数。
const (
	defaultSenderLeaseSecs = 300             // sender 领取批次的租约秒数
	defaultSendRetryDelay  = 5 * time.Minute // 发送失败后默认重试延迟
)

// RootSnapshotResolver 解析事件根对象的展示快照（文章标题/碎语摘要），
// 供邮件正文中标识「哪篇文章/哪条碎语」时按需取用。返回空串不视作错误。
type RootSnapshotResolver interface {
	RootSnapshotOf(ctx context.Context, rootType string, rootID uint) (string, error)
}

// senderRepo 是 sender 依赖的仓储能力子集。
type senderRepo interface {
	notificationrepo.EmailBatchRepository
	notificationrepo.SendLogRepository
	GetEventsByIDs(ctx context.Context, ids []uint) (map[uint]model.NotificationEvent, error)
}

// EmailSender 邮件发送器：领取到点批次，校验额度后渲染发送，并落发送日志与状态。
type EmailSender struct {
	repo         senderRepo
	quota        *QuotaService
	roles        RoleResolver
	roots        RootSnapshotResolver
	mailer       email.MailSender
	provider     string
	siteURL      string
	leaseSeconds int
	retryDelay   time.Duration
	now          func() time.Time
}

// NewEmailSender 创建邮件发送器。
// roots 用于按事件根对象解析展示快照，可为 nil（此时退化为不展示根对象标题）。
// siteURL 为站点前缀，用于文章跳转链接与 Footer；为空时邮件不渲染可点击链接。
func NewEmailSender(repo senderRepo, quota *QuotaService, roles RoleResolver, roots RootSnapshotResolver, mailer email.MailSender, provider, siteURL string) *EmailSender {
	return &EmailSender{
		repo:         repo,
		quota:        quota,
		roles:        roles,
		roots:        roots,
		mailer:       mailer,
		provider:     provider,
		siteURL:      siteURL,
		leaseSeconds: defaultSenderLeaseSecs,
		retryDelay:   defaultSendRetryDelay,
		now:          time.Now,
	}
}

// SendOnce 领取一批到点批次并依次发送，返回成功发送的批次数。
func (s *EmailSender) SendOnce(ctx context.Context, workerID string, limit int) (int, error) {
	batches, err := s.repo.LeaseEmailBatches(ctx, workerID, s.leaseSeconds, limit)
	if err != nil {
		return 0, err
	}

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

// sendBatch 发送单个批次：额度预留 → 渲染 → SMTP → 落状态与日志。
func (s *EmailSender) sendBatch(ctx context.Context, batch model.NotificationEmailBatch) (bool, error) {
	// 发送前先原子预留额度，不足则延后且不调用 SMTP。
	recipientRoles, err := s.roles.Roles(ctx, batch.RecipientUserID)
	if err != nil {
		return false, err
	}
	decision, err := s.quota.Reserve(ctx, QuotaInput{
		Purpose:         batch.Purpose,
		RecipientUserID: batch.RecipientUserID,
		RecipientRoles:  recipientRoles,
		Now:             s.now(),
	})
	if err != nil {
		return false, err
	}
	if !decision.Allowed {
		return false, s.repo.MarkBatchRetry(ctx, batch.ID, decision.DeferUntil, decision.Reason)
	}

	// 渲染摘要：取批次任务与其事件快照。
	tasks, err := s.repo.ListBatchTasks(ctx, batch.ID)
	if err != nil {
		return false, err
	}
	events, err := s.repo.GetEventsByIDs(ctx, eventIDsOf(tasks))
	if err != nil {
		return false, err
	}
	rootLabels, err := s.resolveRootLabels(ctx, events)
	if err != nil {
		return false, err
	}
	htmlBody := renderDigestHTML(tasks, events, rootLabels, s.siteURL)
	messageID := batchMessageID(batch.ID)

	// 调用 SMTP；失败落日志并安排重试。
	if err := s.mailer.SendHTML(batch.ToEmail, batch.Subject, htmlBody, messageID); err != nil {
		_ = s.repo.CreateSendLog(ctx, s.buildSendLog(batch, "failed", messageID, err.Error()))
		return false, s.repo.MarkBatchRetry(ctx, batch.ID, s.now().Add(s.retryDelay), err.Error())
	}

	// 成功：落日志并标记批次与任务为 sent。
	if err := s.repo.CreateSendLog(ctx, s.buildSendLog(batch, "success", messageID, "")); err != nil {
		return false, err
	}
	if err := s.repo.MarkBatchSent(ctx, batch.ID, messageID); err != nil {
		return false, err
	}
	return true, nil
}

// buildSendLog 组装一条发送日志记录。
func (s *EmailSender) buildSendLog(batch model.NotificationEmailBatch, status, messageID, errMsg string) *model.EmailSendLog {
	batchID := batch.ID
	log := &model.EmailSendLog{
		BatchID:   &batchID,
		Purpose:   batch.Purpose,
		ToEmail:   batch.ToEmail,
		Status:    status,
		Provider:  s.provider,
		MessageID: &messageID,
	}
	if errMsg != "" {
		log.Error = &errMsg
	}
	return log
}

// eventIDsOf 去重提取任务引用的事件 ID。
func eventIDsOf(tasks []model.NotificationEmailTask) []uint {
	seen := make(map[uint]struct{}, len(tasks))
	ids := make([]uint, 0, len(tasks))
	for _, task := range tasks {
		if _, ok := seen[task.EventID]; ok {
			continue
		}
		seen[task.EventID] = struct{}{}
		ids = append(ids, task.EventID)
	}
	return ids
}

// batchMessageID 由批次 ID 生成稳定 Message-ID，用于发送侧幂等与追踪。
func batchMessageID(batchID uint) string {
	return fmt.Sprintf("notify-batch-%d", batchID)
}

// resolveRootLabels 按事件 ID 收集根对象展示快照，仅对文章/碎语根做查询；
// 解析器缺失或查询失败时返回空映射，让模板按「ID xx」回退，不阻断邮件发送。
func (s *EmailSender) resolveRootLabels(ctx context.Context, events map[uint]model.NotificationEvent) (map[uint]string, error) {
	if s.roots == nil {
		return nil, nil
	}
	out := make(map[uint]string, len(events))
	for id, event := range events {
		if event.RootID == 0 {
			continue
		}
		switch event.RootType {
		case "article", "moment":
		default:
			continue
		}
		label, err := s.roots.RootSnapshotOf(ctx, event.RootType, event.RootID)
		if err != nil {
			return nil, err
		}
		out[id] = label
	}
	return out, nil
}
