package moment

import "github.com/vpt/blog-backend/internal/model"

func (r *momentRepo) imagesByMomentID(ids []uint) (map[uint][]model.Media, error) {
	result := make(map[uint][]model.Media, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var rows []model.Media
	err := r.db.
		Where("type = ? AND status = ? AND moment_id IN ?", MomentImageType, uint8(1), ids).
		Order("moment_id ASC").
		Order("seq ASC").
		Order("id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.MomentID] = append(result[row.MomentID], row)
	}
	return result, nil
}

func prepareImages(moment model.Moment, images []model.Media) []model.Media {
	prepared := make([]model.Media, 0, len(images))
	for _, image := range images {
		image.MomentID = moment.ID
		image.Type = MomentImageType
		image.UploaderID = moment.UserID
		image.Status = 1
		prepared = append(prepared, image)
	}
	return prepared
}

func removedImageURLs(oldImages []model.Media, newImages []model.Media) []string {
	newURLs := make(map[string]struct{}, len(newImages))
	for _, image := range newImages {
		if image.URL != "" {
			newURLs[image.URL] = struct{}{}
		}
	}

	removed := make([]string, 0)
	seen := map[string]struct{}{}
	for _, image := range oldImages {
		if image.URL == "" {
			continue
		}
		if _, keep := newURLs[image.URL]; keep {
			continue
		}
		if _, exists := seen[image.URL]; exists {
			continue
		}
		seen[image.URL] = struct{}{}
		removed = append(removed, image.URL)
	}
	return removed
}
