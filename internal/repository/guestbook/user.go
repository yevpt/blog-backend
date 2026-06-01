package guestbook

import "github.com/vpt/blog-backend/internal/model"

func (r *guestbookRepo) usersByID(ids []uint) (map[uint]*model.User, error) {
	users := make(map[uint]*model.User, len(ids))
	if len(ids) == 0 {
		return users, nil
	}

	var rows []model.User
	if err := r.db.Where("id IN ?", uniqueUintIDs(ids)).Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		user := rows[i]
		users[user.ID] = &user
	}
	return users, nil
}

func messageIDs(messages []model.Guestbook) []uint {
	ids := make([]uint, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func messageUserIDs(messages []model.Guestbook) []uint {
	ids := make([]uint, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.FromUserID)
	}
	return ids
}

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
