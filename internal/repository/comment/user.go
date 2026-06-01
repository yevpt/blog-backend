package comment

import "github.com/vpt/blog-backend/internal/model"

func (r *commentRepo) usersByID(ids []uint) (map[uint]*model.User, error) {
	uniqueIDs := uniqueUintIDs(ids)
	result := make(map[uint]*model.User, len(uniqueIDs))
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	var users []model.User
	if err := r.db.Where("id IN ?", uniqueIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		user := users[i]
		result[user.ID] = &user
	}
	return result, nil
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
