package moment

import "github.com/vpt/blog-backend/internal/model"

func (r *momentRepo) imagesByMomentID(ids []uint) (map[uint][]model.Media, error) {
	result := make(map[uint][]model.Media, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var rows []model.Media
	err := r.db.
		Where("owner_type = ? AND type = ? AND status = ? AND owner_id IN ?", MomentMediaOwnerType, MomentImageType, uint8(1), ids).
		Order("owner_id ASC").
		Order("seq ASC").
		Order("id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.OwnerID] = append(result[row.OwnerID], row)
	}
	return result, nil
}

func prepareImages(moment model.Moment, images []model.Media) []model.Media {
	prepared := make([]model.Media, 0, len(images))
	for _, image := range images {
		image.OwnerID = moment.ID
		image.OwnerType = MomentMediaOwnerType
		image.Type = MomentImageType
		image.UploaderID = moment.UserID
		image.Status = 1
		prepared = append(prepared, image)
	}
	return prepared
}
