package analytics

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	repo "github.com/vpt/blog-backend/internal/repository/analytics"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

// sessionIngestor 把异步 Ingestor 与同步会话写入合并为 svc.SessionIngestor。
type sessionIngestor struct {
	ing  Ingestor
	repo repo.Repository
}

// NewSessionIngestor 适配 collect service 所需的 SessionIngestor：
// 事件落库走异步 Ingestor，会话写入走同步 repo。
func NewSessionIngestor(ing Ingestor, r repo.Repository) svc.SessionIngestor {
	return &sessionIngestor{ing: ing, repo: r}
}

// Submit 把事件投递给异步落库器。
func (s *sessionIngestor) Submit(ev model.AnalyticsEvent) bool { return s.ing.Submit(ev) }

// UpsertSession 同步写入会话快照。
func (s *sessionIngestor) UpsertSession(ctx context.Context, sess model.AnalyticsSession) error {
	return s.repo.UpsertSession(ctx, sess)
}

// TouchSession 同步刷新会话 last_seen。
func (s *sessionIngestor) TouchSession(ctx context.Context, sessionID string, lastSeen time.Time) error {
	return s.repo.TouchSession(ctx, sessionID, lastSeen)
}
