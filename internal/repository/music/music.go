package music

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// MusicRepository 音乐数据访问接口。
type MusicRepository interface {
	List() ([]model.Music, error)
	ListPublicSongs() ([]model.Music, error)
	FindMusic(id uint) (*model.Music, error)
	MusicArtistRelations(musicIDs []uint) (map[uint][]model.MusicArtist, error)
	ListArtists(keyword string) ([]model.MusicArtist, error)
	FindArtists(ids []uint) ([]model.MusicArtist, error)
	FindArtist(id uint) (*model.MusicArtist, error)
	ListAlbums(keyword string) ([]model.MusicAlbum, error)
	FindAlbum(id uint) (*model.MusicAlbum, error)
	SaveMusic(data MusicSaveData) error
	DeleteMusic(id uint) error
	SaveArtist(data MusicArtistSaveData) (*model.MusicArtist, error)
	DeleteArtist(id uint) error
	SaveAlbum(data MusicAlbumSaveData) (*model.MusicAlbum, error)
	DeleteAlbum(id uint) error
	ListAdminSongs(keyword string, offset, limit int) ([]model.Music, int64, error)
}

// MusicSaveData 保存歌曲及歌手关系所需数据。
type MusicSaveData struct {
	Music           model.Music
	ArtistRelations []model.MusicArtistRelation
	PrepareMusic    func(item model.Music) (model.Music, error)
}

// MusicArtistSaveData 保存歌手所需数据。
type MusicArtistSaveData struct {
	Artist        model.MusicArtist
	PrepareArtist func(artist model.MusicArtist) (model.MusicArtist, error)
}

// MusicAlbumSaveData 保存专辑所需数据。
type MusicAlbumSaveData struct {
	Album        model.MusicAlbum
	PrepareAlbum func(album model.MusicAlbum) (model.MusicAlbum, error)
}

type musicRepo struct {
	db *gorm.DB
}

// NewMusicRepository 创建音乐仓储实例。
func NewMusicRepository(db *gorm.DB) MusicRepository {
	return &musicRepo{db: db}
}

func (r *musicRepo) List() ([]model.Music, error) {
	return r.ListPublicSongs()
}

func (r *musicRepo) ListPublicSongs() ([]model.Music, error) {
	var rows []model.Music
	err := r.db.Model(&model.Music{}).
		Where("is_public = ?", true).
		Order("seq ASC").
		Order("id ASC").
		Find(&rows).Error
	return rows, err
}

func (r *musicRepo) FindMusic(id uint) (*model.Music, error) {
	var item model.Music
	err := r.db.First(&item, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

type musicArtistRelationRow struct {
	MusicID uint `gorm:"column:music_id"`
	model.MusicArtist
}

func (r *musicRepo) MusicArtistRelations(musicIDs []uint) (map[uint][]model.MusicArtist, error) {
	result := make(map[uint][]model.MusicArtist)
	if len(musicIDs) == 0 {
		return result, nil
	}

	var rows []musicArtistRelationRow
	err := r.db.Table("music_artist_relation").
		Select("music_artist_relation.music_id, music_artist.*").
		Joins("JOIN music_artist ON music_artist.id = music_artist_relation.artist_id AND music_artist.deleted_at IS NULL").
		Where("music_artist_relation.music_id IN ?", musicIDs).
		Order("music_artist_relation.seq ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.MusicID] = append(result[row.MusicID], row.MusicArtist)
	}
	return result, nil
}

func (r *musicRepo) ListArtists(keyword string) ([]model.MusicArtist, error) {
	query := r.db.Model(&model.MusicArtist{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR name_zh LIKE ?", like, like)
	}

	var rows []model.MusicArtist
	err := query.Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *musicRepo) FindArtists(ids []uint) ([]model.MusicArtist, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var rows []model.MusicArtist
	err := r.db.Where("id IN ?", ids).Find(&rows).Error
	return rows, err
}

func (r *musicRepo) FindArtist(id uint) (*model.MusicArtist, error) {
	var artist model.MusicArtist
	err := r.db.First(&artist, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &artist, nil
}

func (r *musicRepo) ListAlbums(keyword string) ([]model.MusicAlbum, error) {
	query := r.db.Model(&model.MusicAlbum{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ?", like)
	}

	var rows []model.MusicAlbum
	err := query.Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *musicRepo) FindAlbum(id uint) (*model.MusicAlbum, error) {
	var album model.MusicAlbum
	err := r.db.First(&album, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &album, nil
}

func (r *musicRepo) SaveMusic(data MusicSaveData) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		item := data.Music
		if item.ID == 0 {
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		} else {
			var existing model.Music
			if err := tx.Select("id").First(&existing, item.ID).Error; err != nil {
				return err
			}
		}
		if data.PrepareMusic != nil {
			prepared, err := data.PrepareMusic(item)
			if err != nil {
				return err
			}
			item = prepared
		}
		if item.ID != 0 && (data.Music.ID != 0 || data.PrepareMusic != nil) {
			result := tx.Model(&model.Music{}).Where("id = ?", item.ID).Updates(musicUpdateFields(item))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}

		if err := tx.Where("music_id = ?", item.ID).Delete(&model.MusicArtistRelation{}).Error; err != nil {
			return err
		}
		if len(data.ArtistRelations) == 0 {
			return nil
		}

		relations := make([]model.MusicArtistRelation, 0, len(data.ArtistRelations))
		for _, rel := range data.ArtistRelations {
			rel.ID = 0
			rel.MusicID = item.ID
			relations = append(relations, rel)
		}
		return tx.Create(&relations).Error
	})
}

func musicUpdateFields(item model.Music) map[string]any {
	return map[string]any{
		"name":                item.Name,
		"singer":              item.Singer,
		"artist_display_name": item.ArtistDisplayName,
		"album":               item.Album,
		"album_id":            item.AlbumID,
		"album_track_no":      item.AlbumTrackNo,
		"audio_key":           item.AudioKey,
		"url":                 item.URL,
		"audio_size":          item.AudioSize,
		"audio_mime":          item.AudioMime,
		"audio_hash":          item.AudioHash,
		"cover_img_url":       item.CoverImgUrl,
		"lyric":               item.Lyric,
		"duration":            item.Duration,
		"is_public":           item.IsPublic,
		"seq":                 item.Seq,
	}
}

func (r *musicRepo) DeleteMusic(id uint) error {
	result := r.db.Delete(&model.Music{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *musicRepo) SaveArtist(data MusicArtistSaveData) (*model.MusicArtist, error) {
	var saved model.MusicArtist
	err := r.db.Transaction(func(tx *gorm.DB) error {
		artist := data.Artist
		if artist.ID == 0 {
			if err := tx.Create(&artist).Error; err != nil {
				return err
			}
		} else {
			var existing model.MusicArtist
			if err := tx.Select("id").First(&existing, artist.ID).Error; err != nil {
				return err
			}
		}
		if data.PrepareArtist != nil {
			prepared, err := data.PrepareArtist(artist)
			if err != nil {
				return err
			}
			artist = prepared
		}
		if artist.ID != 0 && (data.Artist.ID != 0 || data.PrepareArtist != nil) {
			result := tx.Model(&model.MusicArtist{}).Where("id = ?", artist.ID).Updates(map[string]any{
				"name":        artist.Name,
				"name_zh":     artist.NameZh,
				"avatar_key":  artist.AvatarKey,
				"description": artist.Description,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		return tx.First(&saved, artist.ID).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (r *musicRepo) DeleteArtist(id uint) error {
	result := r.db.Delete(&model.MusicArtist{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *musicRepo) SaveAlbum(data MusicAlbumSaveData) (*model.MusicAlbum, error) {
	var saved model.MusicAlbum
	err := r.db.Transaction(func(tx *gorm.DB) error {
		album := data.Album
		if album.ID == 0 {
			if err := tx.Create(&album).Error; err != nil {
				return err
			}
		} else {
			var existing model.MusicAlbum
			if err := tx.Select("id").First(&existing, album.ID).Error; err != nil {
				return err
			}
		}
		if data.PrepareAlbum != nil {
			prepared, err := data.PrepareAlbum(album)
			if err != nil {
				return err
			}
			album = prepared
		}
		if album.ID != 0 && (data.Album.ID != 0 || data.PrepareAlbum != nil) {
			result := tx.Model(&model.MusicAlbum{}).Where("id = ?", album.ID).Updates(map[string]any{
				"name":         album.Name,
				"artist_id":    album.ArtistID,
				"cover_key":    album.CoverKey,
				"release_date": album.ReleaseDate,
				"description":  album.Description,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		return tx.First(&saved, album.ID).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (r *musicRepo) DeleteAlbum(id uint) error {
	result := r.db.Delete(&model.MusicAlbum{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *musicRepo) ListAdminSongs(keyword string, offset, limit int) ([]model.Music, int64, error) {
	query := r.db.Model(&model.Music{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			"name LIKE ? OR artist_display_name LIKE ? OR singer LIKE ?",
			like, like, like,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.Music
	err := query.Order("seq ASC").Order("id ASC").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}
