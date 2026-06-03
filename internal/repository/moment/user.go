package moment

import "github.com/vpt/blog-backend/internal/model"

func (r *momentRepo) usersByID(ids []uint) (map[uint]*model.User, error) {
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

func (r *momentRepo) ensureAuthorExists(userID uint) error {
	var count int64
	err := r.db.Model(&model.User{}).
		Where("id = ? AND status = ?", userID, uint8(1)).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrAuthorNotFound
	}
	return nil
}

func momentIDs(moments []model.Moment) []uint {
	ids := make([]uint, 0, len(moments))
	for _, moment := range moments {
		ids = append(ids, moment.ID)
	}
	return ids
}

func momentUserIDs(moments []model.Moment) []uint {
	ids := make([]uint, 0, len(moments))
	for _, moment := range moments {
		ids = append(ids, moment.UserID)
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
