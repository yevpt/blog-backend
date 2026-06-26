package user

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
)

// OnlineChecker 查询注册用户 Redis 在线态（由 analytics.UserPresence 实现）。
type OnlineChecker interface {
	IsUserOnline(ctx context.Context, userID uint) (bool, error)
	BatchIsUserOnline(ctx context.Context, userIDs []uint) (map[uint]bool, error)
}

// enrichListPresence 为列表项批量填充 is_online。
func enrichListPresence(ctx context.Context, presence OnlineChecker, list []dto.UserListItemResp) {
	if presence == nil || len(list) == 0 {
		return
	}
	ids := make([]uint, len(list))
	for i := range list {
		ids[i] = list[i].ID
	}
	online, err := presence.BatchIsUserOnline(ctx, ids)
	if err != nil {
		return
	}
	for i := range list {
		list[i].IsOnline = online[list[i].ID]
	}
}

// enrichDetailPresence 为单用户详情填充 is_online。
func enrichDetailPresence(ctx context.Context, presence OnlineChecker, userID uint, target *bool) {
	if presence == nil || target == nil {
		return
	}
	online, err := presence.IsUserOnline(ctx, userID)
	if err != nil {
		return
	}
	*target = online
}
