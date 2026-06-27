package user

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
)

// ActiveLoginFetcher 查询用户最近活跃/登录时间（由 userrepo.UserRepository 实现）。
type ActiveLoginFetcher interface {
	BatchFetchActiveLogin(ids []uint) (map[uint]*userrepo.ActiveLogin, error)
}

// PresenceProvider 批量返回用户在线状态与最近活跃/登录时间。
type PresenceProvider interface {
	BatchPresence(ctx context.Context, ids []uint) (map[uint]*dto.UserPresenceResp, error)
}

type presenceProvider struct {
	online OnlineChecker
	repo   ActiveLoginFetcher
}

// NewPresenceProvider 创建在线感知聚合服务。
func NewPresenceProvider(online OnlineChecker, repo ActiveLoginFetcher) PresenceProvider {
	return &presenceProvider{online: online, repo: repo}
}

func (p *presenceProvider) BatchPresence(ctx context.Context, ids []uint) (map[uint]*dto.UserPresenceResp, error) {
	if len(ids) == 0 {
		return map[uint]*dto.UserPresenceResp{}, nil
	}

	online, err := p.online.BatchIsUserOnline(ctx, ids)
	if err != nil {
		return nil, err
	}
	activeLogin, err := p.repo.BatchFetchActiveLogin(ids)
	if err != nil {
		return nil, err
	}

	result := make(map[uint]*dto.UserPresenceResp, len(activeLogin))
	for id, al := range activeLogin {
		item := &dto.UserPresenceResp{IsOnline: online[id]}
		if al.LastActiveAt != nil {
			activeAt := al.LastActiveAt.Unix()
			item.LastActiveAt = &activeAt
		}
		if al.LastLoginAt != nil {
			loginAt := al.LastLoginAt.Unix()
			item.LastLoginAt = &loginAt
		}
		result[id] = item
	}
	return result, nil
}
