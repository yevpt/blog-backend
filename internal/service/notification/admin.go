package notification

import (
	"context"
	"errors"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
)

var (
	// ErrQuotaPolicyNotFound 表示要调整的额度策略不存在。
	ErrQuotaPolicyNotFound = errors.New("额度策略不存在")
	// ErrBatchNotRetryable 表示批次不存在或当前状态不可重试。
	ErrBatchNotRetryable = errors.New("批次不存在或不可重试")
)

// AdminService 管理端通知用例：查询邮件任务/批次、调整额度、重试批次。
type AdminService interface {
	ListEmailTasks(req dto.AdminNotificationListReq) (*dto.AdminEmailTaskPageResp, error)
	ListEmailBatches(req dto.AdminNotificationListReq) (*dto.AdminEmailBatchPageResp, error)
	ListQuotas() (*dto.AdminQuotaListResp, error)
	UpdateQuota(id uint, req dto.AdminUpdateQuotaReq) error
	UpdateRoleQuota(id uint, req dto.AdminUpdateRoleQuotaReq) error
	RetryBatch(id uint) (*dto.AdminBatchRetryResp, error)
}

type adminService struct {
	repo notificationrepo.AdminRepository
}

// NewAdminService 创建管理端通知服务。
func NewAdminService(repo notificationrepo.AdminRepository) AdminService {
	return &adminService{repo: repo}
}

// ListEmailTasks 分页查询邮件任务。
func (s *adminService) ListEmailTasks(req dto.AdminNotificationListReq) (*dto.AdminEmailTaskPageResp, error) {
	page, pageSize := normalizePage(req.Page), normalizePageSize(req.PageSize)
	tasks, total, err := s.repo.ListEmailTasks(context.Background(), req.Status, page, pageSize)
	if err != nil {
		return nil, err
	}
	list := make([]dto.AdminEmailTaskResp, 0, len(tasks))
	for _, t := range tasks {
		list = append(list, emailTaskToAdminDTO(t))
	}
	return &dto.AdminEmailTaskPageResp{Total: total, Page: page, PageSize: pageSize, List: list}, nil
}

// ListEmailBatches 分页查询邮件批次。
func (s *adminService) ListEmailBatches(req dto.AdminNotificationListReq) (*dto.AdminEmailBatchPageResp, error) {
	page, pageSize := normalizePage(req.Page), normalizePageSize(req.PageSize)
	batches, total, err := s.repo.ListEmailBatches(context.Background(), req.Status, page, pageSize)
	if err != nil {
		return nil, err
	}
	list := make([]dto.AdminEmailBatchResp, 0, len(batches))
	for _, b := range batches {
		list = append(list, emailBatchToAdminDTO(b))
	}
	return &dto.AdminEmailBatchPageResp{Total: total, Page: page, PageSize: pageSize, List: list}, nil
}

// ListQuotas 读取 purpose 与角色额度策略。
func (s *adminService) ListQuotas() (*dto.AdminQuotaListResp, error) {
	ctx := context.Background()
	policies, err := s.repo.ListQuotaPolicies(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := s.repo.ListRoleQuotaPolicies(ctx)
	if err != nil {
		return nil, err
	}

	resp := &dto.AdminQuotaListResp{
		Purposes: make([]dto.AdminQuotaPolicyResp, 0, len(policies)),
		Roles:    make([]dto.AdminRoleQuotaPolicyResp, 0, len(roles)),
	}
	for _, p := range policies {
		resp.Purposes = append(resp.Purposes, dto.AdminQuotaPolicyResp{
			ID: p.ID, Purpose: p.Purpose, DailyLimit: p.DailyLimit, ReservedMin: p.ReservedMin,
			Priority: p.Priority, MaxPerMinute: p.MaxPerMinute, MaxPerHour: p.MaxPerHour, Enabled: p.Enabled,
		})
	}
	for _, r := range roles {
		resp.Roles = append(resp.Roles, dto.AdminRoleQuotaPolicyResp{
			ID: r.ID, Role: r.Role, ScopeType: r.ScopeType, DailyLimit: r.DailyLimit, MaxPerHour: r.MaxPerHour, Enabled: r.Enabled,
		})
	}
	return resp, nil
}

// UpdateQuota 调整 purpose 额度策略。
func (s *adminService) UpdateQuota(id uint, req dto.AdminUpdateQuotaReq) error {
	fields := map[string]any{
		"daily_limit":    req.DailyLimit,
		"reserved_min":   req.ReservedMin,
		"priority":       req.Priority,
		"max_per_minute": req.MaxPerMinute,
		"max_per_hour":   req.MaxPerHour,
	}
	if req.Enabled != nil {
		fields["enabled"] = *req.Enabled
	}
	affected, err := s.repo.UpdateQuotaPolicy(context.Background(), id, fields)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrQuotaPolicyNotFound
	}
	return nil
}

// UpdateRoleQuota 调整角色额度策略。
func (s *adminService) UpdateRoleQuota(id uint, req dto.AdminUpdateRoleQuotaReq) error {
	fields := map[string]any{
		"daily_limit":  req.DailyLimit,
		"max_per_hour": req.MaxPerHour,
	}
	if req.Enabled != nil {
		fields["enabled"] = *req.Enabled
	}
	affected, err := s.repo.UpdateRoleQuotaPolicy(context.Background(), id, fields)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrQuotaPolicyNotFound
	}
	return nil
}

// RetryBatch 把失败/延后批次重置为 pending；不存在或状态不可重试时报错。
func (s *adminService) RetryBatch(id uint) (*dto.AdminBatchRetryResp, error) {
	affected, err := s.repo.RetryBatch(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrBatchNotRetryable
	}
	return &dto.AdminBatchRetryResp{ID: id, Status: notificationrepo.EmailBatchStatusPending}, nil
}

// emailTaskToAdminDTO 把任务模型映射为管理端 DTO。
func emailTaskToAdminDTO(t model.NotificationEmailTask) dto.AdminEmailTaskResp {
	return dto.AdminEmailTaskResp{
		ID: t.ID, EventID: t.EventID, RecipientUserID: t.RecipientUserID, ActorUserID: t.ActorUserID,
		ToEmail: t.ToEmail, EventType: t.EventType, Purpose: t.Purpose, Status: t.Status,
		Attempts: t.Attempts, BatchID: t.BatchID, CreatedAt: t.CreatedAt,
	}
}

// emailBatchToAdminDTO 把批次模型映射为管理端 DTO。
func emailBatchToAdminDTO(b model.NotificationEmailBatch) dto.AdminEmailBatchResp {
	return dto.AdminEmailBatchResp{
		ID: b.ID, RecipientUserID: b.RecipientUserID, ToEmail: b.ToEmail, Purpose: b.Purpose,
		Subject: b.Subject, Status: b.Status, ItemCount: b.ItemCount, Attempts: b.Attempts,
		ScheduledAt: b.ScheduledAt, SentAt: b.SentAt, CreatedAt: b.CreatedAt,
	}
}
