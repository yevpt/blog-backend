package notification

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
)

// 额度维度、窗口与通配符。
const (
	quotaScopeSite      = "site"
	quotaScopePurpose   = "purpose"
	quotaScopeActor     = "actor"
	quotaScopeRecipient = "recipient"
	quotaWildcard       = "*"

	windowDay    = "day"
	windowHour   = "hour"
	windowMinute = "minute"
)

// QuotaConfig 站点级安全限额，来自 email 配置。
type QuotaConfig struct {
	SiteDailySafeLimit int // 全站每日安全上限
	MaxPerMinute       int // 全站每分钟上限
	MaxPerHour         int // 全站每小时上限
}

// QuotaInput 一次额度评估的输入。
type QuotaInput struct {
	Purpose         string    // 邮件用途
	ActorUserID     uint      // 操作人
	RecipientUserID uint      // 接收人
	ActorRoles      []string  // 操作人角色
	RecipientRoles  []string  // 接收人角色
	Now             time.Time // 评估时刻
}

// QuotaDecision 额度评估结果。
type QuotaDecision struct {
	Allowed    bool      // 是否允许发送
	DeferUntil time.Time // 不允许时建议的下次重试时间
	Reason     string    // 不允许的原因
}

// QuotaStore 额度服务所需的策略、用量读取与原子占用能力。
type QuotaStore interface {
	GetQuotaPolicies(ctx context.Context) ([]model.EmailQuotaPolicy, error)
	GetRoleQuotaPolicies(ctx context.Context) ([]model.EmailRoleQuotaPolicy, error)
	GetUsage(ctx context.Context, key notificationrepo.QuotaUsageKey) (int, error)
	ReserveQuota(ctx context.Context, key notificationrepo.QuotaUsageKey, limit int) (bool, error)
}

// QuotaService 邮件额度评估器。
//
// 分四层判断，任一层不足即拒绝并给出建议重试时间：
//  1. 全站安全额度（每日/每分钟/每小时）；
//  2. purpose 额度，并为其它 purpose 的保底份额预留全站额度；
//  3. actor 角色每日/每小时额度；
//  4. recipient 角色每日额度。
type QuotaService struct {
	store QuotaStore
	cfg   QuotaConfig
}

// NewQuotaService 创建额度评估器。
func NewQuotaService(store QuotaStore, cfg QuotaConfig) *QuotaService {
	return &QuotaService{store: store, cfg: cfg}
}

// Evaluate 评估一次发送是否被额度允许。
func (s *QuotaService) Evaluate(ctx context.Context, in QuotaInput) (QuotaDecision, error) {
	policies, err := s.store.GetQuotaPolicies(ctx)
	if err != nil {
		return QuotaDecision{}, err
	}
	roleQuotas, err := s.store.GetRoleQuotaPolicies(ctx)
	if err != nil {
		return QuotaDecision{}, err
	}

	now := in.Now
	dayStart := startOfDay(now)
	hourStart := now.Truncate(time.Hour)
	minuteStart := now.Truncate(time.Minute)

	// 第一层：全站安全额度，并为其它 purpose 的保底份额预留。
	siteLimit := s.cfg.SiteDailySafeLimit - reservedForOthers(policies, in.Purpose)
	if d, blocked, err := s.check(ctx, siteKey(quotaWildcard, windowDay, dayStart), siteLimit, dayStart.Add(24*time.Hour), "全站每日安全额度不足"); err != nil || blocked {
		return d, err
	}
	if d, blocked, err := s.check(ctx, siteKey(quotaWildcard, windowMinute, minuteStart), s.cfg.MaxPerMinute, minuteStart.Add(time.Minute), "全站每分钟发送上限"); err != nil || blocked {
		return d, err
	}
	if d, blocked, err := s.check(ctx, siteKey(quotaWildcard, windowHour, hourStart), s.cfg.MaxPerHour, hourStart.Add(time.Hour), "全站每小时发送上限"); err != nil || blocked {
		return d, err
	}

	// 第二层：purpose 额度。
	if policy, ok := enabledPolicy(policies, in.Purpose); ok {
		if d, blocked, err := s.check(ctx, purposeKey(in.Purpose, windowDay, dayStart), policy.DailyLimit, dayStart.Add(24*time.Hour), "该用途每日额度不足"); err != nil || blocked {
			return d, err
		}
		if policy.MaxPerMinute > 0 {
			if d, blocked, err := s.check(ctx, purposeKey(in.Purpose, windowMinute, minuteStart), policy.MaxPerMinute, minuteStart.Add(time.Minute), "该用途每分钟上限"); err != nil || blocked {
				return d, err
			}
		}
		if policy.MaxPerHour > 0 {
			if d, blocked, err := s.check(ctx, purposeKey(in.Purpose, windowHour, hourStart), policy.MaxPerHour, hourStart.Add(time.Hour), "该用途每小时上限"); err != nil || blocked {
				return d, err
			}
		}
	}

	// 第三层：actor 角色额度。
	if limit := maxRoleLimit(roleQuotas, in.ActorRoles, quotaScopeActor, false); limit > 0 {
		if d, blocked, err := s.check(ctx, userKey(quotaScopeActor, in.ActorUserID, windowDay, dayStart), limit, dayStart.Add(24*time.Hour), "操作人每日通知邮件额度不足"); err != nil || blocked {
			return d, err
		}
	}
	if limit := maxRoleLimit(roleQuotas, in.ActorRoles, quotaScopeActor, true); limit > 0 {
		if d, blocked, err := s.check(ctx, userKey(quotaScopeActor, in.ActorUserID, windowHour, hourStart), limit, hourStart.Add(time.Hour), "操作人每小时通知邮件上限"); err != nil || blocked {
			return d, err
		}
	}

	// 第四层：recipient 角色额度。
	if limit := maxRoleLimit(roleQuotas, in.RecipientRoles, quotaScopeRecipient, false); limit > 0 {
		if d, blocked, err := s.check(ctx, userKey(quotaScopeRecipient, in.RecipientUserID, windowDay, dayStart), limit, dayStart.Add(24*time.Hour), "接收人每日通知邮件额度不足"); err != nil || blocked {
			return d, err
		}
	}

	return QuotaDecision{Allowed: true}, nil
}

// Reserve 在确认可发送后原子占用各维度额度。
//
// 逐维度用「未达上限才自增」的条件更新占额；任一维度占用失败即返回延后决策。
// 调用方应在评估通过后、真正发送前调用；多 worker 下条件自增保证不会超发。
func (s *QuotaService) Reserve(ctx context.Context, in QuotaInput) (QuotaDecision, error) {
	policies, err := s.store.GetQuotaPolicies(ctx)
	if err != nil {
		return QuotaDecision{}, err
	}
	roleQuotas, err := s.store.GetRoleQuotaPolicies(ctx)
	if err != nil {
		return QuotaDecision{}, err
	}

	now := in.Now
	dayStart := startOfDay(now)
	hourStart := now.Truncate(time.Hour)
	minuteStart := now.Truncate(time.Minute)

	// 占用顺序与评估一致：全站 → purpose → actor → recipient。
	reservations := []struct {
		key        notificationrepo.QuotaUsageKey
		limit      int
		deferUntil time.Time
		reason     string
	}{
		{siteKey(quotaWildcard, windowDay, dayStart), s.cfg.SiteDailySafeLimit - reservedForOthers(policies, in.Purpose), dayStart.Add(24 * time.Hour), "全站每日安全额度不足"},
		{siteKey(quotaWildcard, windowMinute, minuteStart), s.cfg.MaxPerMinute, minuteStart.Add(time.Minute), "全站每分钟发送上限"},
		{siteKey(quotaWildcard, windowHour, hourStart), s.cfg.MaxPerHour, hourStart.Add(time.Hour), "全站每小时发送上限"},
	}
	if policy, ok := enabledPolicy(policies, in.Purpose); ok {
		reservations = append(reservations, struct {
			key        notificationrepo.QuotaUsageKey
			limit      int
			deferUntil time.Time
			reason     string
		}{purposeKey(in.Purpose, windowDay, dayStart), policy.DailyLimit, dayStart.Add(24 * time.Hour), "该用途每日额度不足"})
	}
	if limit := maxRoleLimit(roleQuotas, in.ActorRoles, quotaScopeActor, false); limit > 0 {
		reservations = append(reservations, struct {
			key        notificationrepo.QuotaUsageKey
			limit      int
			deferUntil time.Time
			reason     string
		}{userKey(quotaScopeActor, in.ActorUserID, windowDay, dayStart), limit, dayStart.Add(24 * time.Hour), "操作人每日通知邮件额度不足"})
	}
	if limit := maxRoleLimit(roleQuotas, in.RecipientRoles, quotaScopeRecipient, false); limit > 0 {
		reservations = append(reservations, struct {
			key        notificationrepo.QuotaUsageKey
			limit      int
			deferUntil time.Time
			reason     string
		}{userKey(quotaScopeRecipient, in.RecipientUserID, windowDay, dayStart), limit, dayStart.Add(24 * time.Hour), "接收人每日通知邮件额度不足"})
	}

	for _, r := range reservations {
		ok, err := s.store.ReserveQuota(ctx, r.key, r.limit)
		if err != nil {
			return QuotaDecision{}, err
		}
		if !ok {
			return QuotaDecision{Allowed: false, DeferUntil: r.deferUntil, Reason: r.reason}, nil
		}
	}
	return QuotaDecision{Allowed: true}, nil
}

// check 读取某额度键用量并与上限比较；超限返回拒绝决策。
// limit <= 0 视为该维度已无可用额度（如保底预留把全站额度挤为非正）。
func (s *QuotaService) check(ctx context.Context, key notificationrepo.QuotaUsageKey, limit int, deferUntil time.Time, reason string) (QuotaDecision, bool, error) {
	used, err := s.store.GetUsage(ctx, key)
	if err != nil {
		return QuotaDecision{}, false, err
	}
	if used+1 > limit {
		return QuotaDecision{Allowed: false, DeferUntil: deferUntil, Reason: reason}, true, nil
	}
	return QuotaDecision{Allowed: true}, false, nil
}

// reservedForOthers 汇总除当前 purpose 外其它 purpose 的保底份额，用于压低当前 purpose 的全站可用额度。
func reservedForOthers(policies []model.EmailQuotaPolicy, purpose string) int {
	total := 0
	for _, p := range policies {
		if p.Enabled && p.Purpose != purpose {
			total += p.ReservedMin
		}
	}
	return total
}

// enabledPolicy 查找启用的 purpose 策略。
func enabledPolicy(policies []model.EmailQuotaPolicy, purpose string) (model.EmailQuotaPolicy, bool) {
	for _, p := range policies {
		if p.Purpose == purpose && p.Enabled {
			return p, true
		}
	}
	return model.EmailQuotaPolicy{}, false
}

// maxRoleLimit 取用户多个角色在某维度下的最高额度；hour 为 true 取每小时上限，否则取每日上限。
func maxRoleLimit(roleQuotas []model.EmailRoleQuotaPolicy, roles []string, scope string, hour bool) int {
	roleSet := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		roleSet[r] = struct{}{}
	}
	limit := 0
	for _, q := range roleQuotas {
		if !q.Enabled || q.ScopeType != scope {
			continue
		}
		if _, ok := roleSet[q.Role]; !ok {
			continue
		}
		value := q.DailyLimit
		if hour {
			value = q.MaxPerHour
		}
		if value > limit {
			limit = value
		}
	}
	return limit
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func siteKey(purpose, window string, start time.Time) notificationrepo.QuotaUsageKey {
	return notificationrepo.QuotaUsageKey{
		QuotaDate:   startOfDay(start),
		ScopeType:   quotaScopeSite,
		ScopeID:     0,
		Purpose:     purpose,
		WindowType:  window,
		WindowStart: start,
	}
}

func purposeKey(purpose, window string, start time.Time) notificationrepo.QuotaUsageKey {
	return notificationrepo.QuotaUsageKey{
		QuotaDate:   startOfDay(start),
		ScopeType:   quotaScopePurpose,
		ScopeID:     0,
		Purpose:     purpose,
		WindowType:  window,
		WindowStart: start,
	}
}

func userKey(scope string, userID uint, window string, start time.Time) notificationrepo.QuotaUsageKey {
	return notificationrepo.QuotaUsageKey{
		QuotaDate:   startOfDay(start),
		ScopeType:   scope,
		ScopeID:     userID,
		Purpose:     quotaWildcard,
		WindowType:  window,
		WindowStart: start,
	}
}
