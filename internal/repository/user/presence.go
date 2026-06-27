package user

import (
	"time"

	"github.com/vpt/blog-backend/internal/model"
)

// ActiveLogin 用户最近活跃/登录时间，供在线感知端点组装响应使用。
type ActiveLogin struct {
	LastActiveAt *time.Time
	LastLoginAt  *time.Time
}

// BatchFetchActiveLogin 按 id 批量查询用户最近活跃/登录时间；未知 id 不在返回的 map 中。
func (r *userRepo) BatchFetchActiveLogin(ids []uint) (map[uint]*ActiveLogin, error) {
	result := make(map[uint]*ActiveLogin, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	type activeLoginRow struct {
		ID           uint
		LastActiveAt *time.Time
		LastLoginAt  *time.Time
	}
	var rows []activeLoginRow
	err := r.db.Model(&model.User{}).
		Select("id, last_active_at, last_login_at").
		Where("id IN ?", ids).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.ID] = &ActiveLogin{LastActiveAt: row.LastActiveAt, LastLoginAt: row.LastLoginAt}
	}
	return result, nil
}
