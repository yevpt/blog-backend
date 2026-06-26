// Package dashboard 组装后台首页汇总数据。
package dashboard

import (
	"context"
	"time"

	dto "github.com/vpt/blog-backend/internal/dto/dashboard"
	repo "github.com/vpt/blog-backend/internal/repository/dashboard"
)

// recentDays 近期互动统计窗口（天）。
const recentDays = 7

// Service 提供后台首页汇总。
type Service interface {
	Overview(ctx context.Context) (dto.OverviewSummary, error)
}

type service struct {
	repo repo.Repository
	tz   *time.Location
}

// NewService 构造首页汇总服务。tz 用于「今日」切天，与统计口径一致。
func NewService(repo repo.Repository, tz *time.Location) Service {
	if tz == nil {
		tz = time.UTC
	}
	return &service{repo: repo, tz: tz}
}

func (s *service) Overview(ctx context.Context) (dto.OverviewSummary, error) {
	now := time.Now().In(s.tz)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.tz)
	since := now.AddDate(0, 0, -recentDays)

	c, err := s.repo.Summary(ctx, since, dayStart)
	if err != nil {
		return dto.OverviewSummary{}, err
	}
	return dto.OverviewSummary{
		Content: dto.ContentCounts{
			Articles:    c.Articles,
			Categories:  c.Categories,
			Tags:        c.Tags,
			Music:       c.Music,
			FriendLinks: c.FriendLinks,
		},
		Interactions: dto.InteractionCounts{
			NewComments:  c.NewComments,
			NewGuestbook: c.NewGuestbook,
			NewMoments:   c.NewMoments,
		},
		Users: dto.UserCounts{
			Total:       c.UsersTotal,
			TodayNew:    c.UsersTodayNew,
			TodayActive: c.UsersTodayActive,
		},
	}, nil
}
