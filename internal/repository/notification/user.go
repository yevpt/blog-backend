package notification

import (
	"context"

	"github.com/vpt/blog-backend/internal/model"
)

// usersByID 按 ID 批量取用户，返回以用户 ID 为键的映射，供列表聚合填充操作人摘要。
func (r *repo) usersByID(ctx context.Context, ids []uint) (map[uint]*model.User, error) {
	uniqueIDs := uniqueUintIDs(ids)
	result := make(map[uint]*model.User, len(uniqueIDs))
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	var users []model.User
	if err := r.db.WithContext(ctx).Where("id IN ?", uniqueIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		user := users[i]
		result[user.ID] = &user
	}
	return result, nil
}

// uniqueUintIDs 去重收集非零 ID，避免批量查询重复入参。
func uniqueUintIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

// actorUserOf 从用户映射中取事件操作人，系统通知（ActorUserID 为空）返回 nil。
func actorUserOf(event model.NotificationEvent, users map[uint]*model.User) *model.User {
	if event.ActorUserID == nil {
		return nil
	}
	return users[*event.ActorUserID]
}
