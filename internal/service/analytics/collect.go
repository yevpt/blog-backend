package analytics

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"go.uber.org/zap"
)

// SessionIngestor 抽象事件落库与会话维护（由 worker + repo 适配）。
type SessionIngestor interface {
	Submit(ev model.AnalyticsEvent) bool
	UpsertSession(ctx context.Context, s model.AnalyticsSession) error
	TouchSession(ctx context.Context, sessionID string, lastSeen time.Time) error
}

// DedupChecker 判断同一访客/会话/路径短窗口内是否重复 PV。
type DedupChecker interface {
	IsDuplicatePV(ctx context.Context, visitorID, sessionID, path string) (bool, error)
}

// CollectService 编排上报：富化 → 实时层 → 去重 → 入库/会话。
type CollectService interface {
	Handle(ctx context.Context, raw RawEvent) error
}

type collectService struct {
	enricher      Enricher
	realtime      Realtime
	ingestor      SessionIngestor
	dedup         DedupChecker
	tokenVerifier CollectTokenVerifier
	logger        *zap.Logger
}

// NewCollectService 注入富化、实时、入库、去重、token 校验依赖，构造编排服务。
func NewCollectService(enricher Enricher, realtime Realtime, ingestor SessionIngestor, dedup DedupChecker, tokenVerifier CollectTokenVerifier, logger *zap.Logger) CollectService {
	return &collectService{enricher: enricher, realtime: realtime, ingestor: ingestor, dedup: dedup, tokenVerifier: tokenVerifier, logger: logger}
}

// Handle 编排单次上报：富化 → 刷新在线 → 按事件类型分支处理。
// 非关键路径（实时层、入库、会话）失败仅 Warn，不阻断上报。
func (s *collectService) Handle(ctx context.Context, raw RawEvent) error {
	// 先做 suspect 决策再富化：后续在线/今日计数均以 IsSuspect 门控，须在 Enrich 前写入 raw。
	// 心跳不校验 collect token：SSR token 5 分钟过期且页面不刷新，长会话心跳会携带过期 token；
	// 而 PV/UV 才是值得防伪造的指标，心跳仅维持在线/时长。page_view 仍完整校验。
	tokenOK, tokenReason := true, ""
	if raw.EventType != "heartbeat" {
		tokenOK, tokenReason = s.tokenVerifier.Verify(raw.CollectToken)
	}
	raw.IsSuspect, raw.SuspectReason = DecideSuspect(raw, tokenOK, tokenReason)

	ev := s.enricher.Enrich(raw)
	now := time.Now()
	ev.CreatedAt = now

	// bot/伪造来源不计在线，仅可信真人刷新在线表（与历史聚合 is_suspect=0 口径一致）。
	if !ev.IsBot && !ev.IsSuspect {
		if err := s.realtime.TouchOnline(ctx, ev.VisitorID); err != nil {
			s.logger.Warn("刷新在线失败", zap.Error(err))
		}
	}

	// 心跳：仅刷新会话 last_seen，不入事件表、不计今日。
	if ev.EventType == "heartbeat" {
		if err := s.ingestor.TouchSession(ctx, ev.SessionID, now); err != nil {
			s.logger.Warn("心跳更新会话失败", zap.Error(err))
		}
		return nil
	}

	// page_view：去重命中则跳过入库与计数（在线已刷新）。
	dup := false
	if d, err := s.dedup.IsDuplicatePV(ctx, ev.VisitorID, ev.SessionID, ev.Path); err != nil {
		s.logger.Warn("去重判定失败", zap.Error(err))
	} else {
		dup = d
	}
	if dup {
		return nil
	}

	// 入库与会话 upsert（bot 也入库，带 is_bot 标记便于审计）。
	s.ingestor.Submit(ev)
	if err := s.ingestor.UpsertSession(ctx, sessionFrom(ev, now)); err != nil {
		s.logger.Warn("会话 upsert 失败", zap.Error(err))
	}

	// 今日计数仅非 bot、非伪造来源且非重复（dup 已早返回，此处恒为非重复），与历史聚合口径一致。
	if !ev.IsBot && !ev.IsSuspect && !dup {
		if err := s.realtime.IncrToday(ctx, ev); err != nil {
			s.logger.Warn("今日计数失败", zap.Error(err))
		}
	}
	return nil
}

// sessionFrom 由富化后的事件构造一次会话快照（首末路径同一、PV 计 1）。
func sessionFrom(ev model.AnalyticsEvent, now time.Time) model.AnalyticsSession {
	return model.AnalyticsSession{
		SessionID:       ev.SessionID,
		VisitorID:       ev.VisitorID,
		UserID:          ev.UserID,
		IsAuthenticated: ev.IsAuthenticated,
		FirstSeen:       now,
		LastSeen:        now,
		PVCount:         1,
		IsBounce:        true,
		EntryPath:       ev.Path,
		ExitPath:        ev.Path,
		DeviceType:      ev.DeviceType,
		Browser:         ev.Browser,
		OS:              ev.OS,
		Country:         ev.Country,
		Region:          ev.Region,
		RefererType:     ev.RefererType,
		IsBot:           ev.IsBot,
	}
}
