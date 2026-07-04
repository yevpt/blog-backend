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
	enrichListPresenceBy(ctx, presence, len(list), func(i int) uint { return list[i].ID }, func(i int, online bool) {
		list[i].IsOnline = online
	})
}

// enrichListPresenceBy 是批量填充在线态的通用实现：由调用方提供取 ID 与写回结果的方式，
// 从而适配不同的列表项 DTO（公开列表、管理端列表等），避免为每种 DTO 各写一份重复的批量查询逻辑。
func enrichListPresenceBy(ctx context.Context, presence OnlineChecker, n int, idAt func(int) uint, setOnline func(int, bool)) {
	if presence == nil || n == 0 {
		return
	}
	ids := make([]uint, n)
	for i := 0; i < n; i++ {
		ids[i] = idAt(i)
	}
	online, err := presence.BatchIsUserOnline(ctx, ids)
	if err != nil {
		return
	}
	for i := 0; i < n; i++ {
		setOnline(i, online[idAt(i)])
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
