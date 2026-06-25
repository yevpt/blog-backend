package music

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	musicrepo "github.com/vpt/blog-backend/internal/repository/music"
	"github.com/vpt/blog-backend/pkg/storage"
	"gorm.io/gorm"
)

var (
	ErrMusicNotFound       = errors.New("音乐不存在")
	ErrMusicArtistNotFound = errors.New("歌手不存在")
	ErrMusicAlbumNotFound  = errors.New("专辑不存在")
)

// MusicService 音乐业务接口。
type MusicService interface {
	List() (*dto.MusicListResp, error)
	ListPublic() (*dto.MusicListResp, error)
	GetPublicDetail(id uint) (*dto.MusicDetailResp, error)
	ListArtists(keyword string) (*dto.MusicArtistListResp, error)
	GetPublicArtist(id uint) (*dto.MusicArtistResp, error)
	ListAlbums(keyword string) (*dto.MusicAlbumListResp, error)
	GetPublicAlbum(id uint) (*dto.MusicAlbumResp, error)
	ListAdmin(req dto.MusicAdminListReq) (*dto.MusicAdminListResp, error)
	SaveMusic(ctx context.Context, userID uint, req dto.MusicSaveReq) error
	DeleteMusic(id uint) error
	SaveArtist(ctx context.Context, userID uint, req dto.MusicArtistSaveReq) (*dto.MusicArtistResp, error)
	DeleteArtist(id uint) error
	SaveAlbum(ctx context.Context, userID uint, req dto.MusicAlbumSaveReq) (*dto.MusicAlbumResp, error)
	DeleteAlbum(id uint) error
	UploadAudio(ctx context.Context, input MusicAudioUploadInput) (*dto.MusicUploadResp, error)
	UploadAlbumCover(ctx context.Context, input MusicImageUploadInput) (*dto.MusicUploadResp, error)
	UploadArtistAvatar(ctx context.Context, input MusicImageUploadInput) (*dto.MusicUploadResp, error)
}

type musicService struct {
	repo  musicrepo.MusicRepository
	store storage.ObjectStore
}

// NewMusicService 创建音乐业务服务实例。
func NewMusicService(repo musicrepo.MusicRepository, store storage.ObjectStore) MusicService {
	return &musicService{repo: repo, store: store}
}

func (s *musicService) List() (*dto.MusicListResp, error) {
	return s.ListPublic()
}

func (s *musicService) ListPublic() (*dto.MusicListResp, error) {
	rows, err := s.repo.ListPublicSongs()
	if err != nil {
		return nil, err
	}
	return s.musicListToDTO(rows)
}

func (s *musicService) GetPublicDetail(id uint) (*dto.MusicDetailResp, error) {
	item, err := s.repo.FindMusic(id)
	if err != nil {
		return nil, err
	}
	if item == nil || !item.IsPublic {
		return nil, ErrMusicNotFound
	}

	relations, err := s.repo.MusicArtistRelations([]uint{id})
	if err != nil {
		return nil, err
	}

	albumIDs := collectMusicAlbumIDs([]model.Music{*item})
	albums, artistsByID, err := s.loadAlbumContext(albumIDs, relations)
	if err != nil {
		return nil, err
	}

	itemDTO := s.musicItemToDTO(item, relations, albums, artistsByID)
	return &dto.MusicDetailResp{
		MusicItemResp: itemDTO,
		Lyric:         item.Lyric,
	}, nil
}

func (s *musicService) ListArtists(keyword string) (*dto.MusicArtistListResp, error) {
	rows, err := s.repo.ListArtists(keyword)
	if err != nil {
		return nil, err
	}

	list := make([]dto.MusicArtistResp, 0, len(rows))
	for i := range rows {
		list = append(list, s.artistToDTO(&rows[i]))
	}
	return &dto.MusicArtistListResp{List: list}, nil
}

func (s *musicService) GetPublicArtist(id uint) (*dto.MusicArtistResp, error) {
	artist, err := s.repo.FindArtist(id)
	if err != nil {
		return nil, err
	}
	if artist == nil {
		return nil, ErrMusicArtistNotFound
	}
	resp := s.artistToDTO(artist)
	return &resp, nil
}

func (s *musicService) ListAlbums(keyword string) (*dto.MusicAlbumListResp, error) {
	rows, err := s.repo.ListAlbums(keyword)
	if err != nil {
		return nil, err
	}

	artistIDs := make([]uint, 0)
	seen := make(map[uint]struct{})
	for i := range rows {
		if rows[i].ArtistID == nil {
			continue
		}
		if _, ok := seen[*rows[i].ArtistID]; ok {
			continue
		}
		seen[*rows[i].ArtistID] = struct{}{}
		artistIDs = append(artistIDs, *rows[i].ArtistID)
	}

	artistsByID, err := s.artistsByID(artistIDs)
	if err != nil {
		return nil, err
	}

	list := make([]dto.MusicAlbumResp, 0, len(rows))
	for i := range rows {
		list = append(list, *s.albumToDTO(&rows[i], artistsByID))
	}
	return &dto.MusicAlbumListResp{List: list}, nil
}

func (s *musicService) GetPublicAlbum(id uint) (*dto.MusicAlbumResp, error) {
	album, err := s.repo.FindAlbum(id)
	if err != nil {
		return nil, err
	}
	if album == nil {
		return nil, ErrMusicAlbumNotFound
	}

	artistsByID := make(map[uint]model.MusicArtist)
	if album.ArtistID != nil {
		artistsByID, err = s.artistsByID([]uint{*album.ArtistID})
		if err != nil {
			return nil, err
		}
	}
	return s.albumToDTO(album, artistsByID), nil
}

func (s *musicService) ListAdmin(req dto.MusicAdminListReq) (*dto.MusicAdminListResp, error) {
	offset := (req.Page - 1) * req.PageSize
	rows, total, err := s.repo.ListAdminSongs(strings.TrimSpace(req.Keyword), offset, req.PageSize)
	if err != nil {
		return nil, err
	}

	listResp, err := s.musicListToDTO(rows)
	if err != nil {
		return nil, err
	}
	return &dto.MusicAdminListResp{
		List:  listResp.List,
		Total: total,
	}, nil
}

func (s *musicService) SaveMusic(ctx context.Context, userID uint, req dto.MusicSaveReq) error {
	uniqueIDs := uniqueArtistIDs(req.ArtistIDs)
	artists, err := s.repo.FindArtists(uniqueIDs)
	if err != nil {
		return err
	}
	if len(artists) != len(uniqueIDs) {
		return ErrMusicArtistNotFound
	}

	legacyAlbum := ""
	var legacyCover *string
	if req.AlbumID != nil {
		album, err := s.repo.FindAlbum(*req.AlbumID)
		if err != nil {
			return err
		}
		if album == nil {
			return ErrMusicAlbumNotFound
		}
		legacyAlbum = album.Name
		legacyCover = album.CoverKey
	}

	artistsByID, err := s.artistsByID(uniqueIDs)
	if err != nil {
		return err
	}

	displayName := strings.TrimSpace(req.ArtistDisplayName)
	if displayName == "" {
		names := make([]string, 0, len(req.ArtistIDs))
		for _, artistID := range req.ArtistIDs {
			artist, ok := artistsByID[artistID]
			if !ok {
				continue
			}
			names = append(names, ArtistDisplayName(artist.Name, artist.NameZh))
		}
		displayName = strings.Join(names, " / ")
	}

	audioKey := req.AudioKey
	relations := make([]model.MusicArtistRelation, 0, len(req.ArtistIDs))
	for seq, artistID := range req.ArtistIDs {
		relations = append(relations, model.MusicArtistRelation{
			ArtistID: artistID,
			Role:     "primary",
			Seq:      uint(seq),
		})
	}

	var oldAudioKey *string
	if req.ID > 0 {
		existing, findErr := s.repo.FindMusic(req.ID)
		if findErr != nil {
			return findErr
		}
		if existing == nil {
			return ErrMusicNotFound
		}
		oldAudioKey = existing.AudioKey
		if oldAudioKey == nil || strings.TrimSpace(*oldAudioKey) == "" {
			oldAudioKey = existing.URL
		}
	}

	var copiedKeys []string
	var tempKey string
	var normalizedAudio string

	item := model.Music{
		Base:              model.Base{ID: req.ID},
		Name:              req.Name,
		Singer:            displayName,
		ArtistDisplayName: displayName,
		Album:             legacyAlbum,
		AlbumID:           req.AlbumID,
		AlbumTrackNo:      req.AlbumTrackNo,
		AudioKey:          &audioKey,
		URL:               &audioKey,
		AudioSize:         req.AudioSize,
		AudioMime:         req.AudioMime,
		AudioHash:         req.AudioHash,
		CoverImgUrl:       legacyCover,
		Lyric:             req.Lyric,
		Duration:          req.Duration,
		IsPublic:          req.IsPublic,
		Seq:               req.Seq,
	}
	err = s.repo.SaveMusic(musicrepo.MusicSaveData{
		Music:           item,
		ArtistRelations: relations,
		PrepareMusic: func(saved model.Music) (model.Music, error) {
			result, normalizeErr := normalizeMusicAudioKey(ctx, s.store, userID, saved.ID, req.AudioKey, oldAudioKey)
			if normalizeErr != nil {
				return model.Music{}, normalizeErr
			}
			copiedKeys = result.CopiedKeys
			tempKey = result.TempKey
			normalizedAudio = result.Key
			saved.AudioKey = &result.Key
			saved.URL = &result.Key
			return saved, nil
		},
	})
	if err != nil {
		_ = s.deleteMusicAssetKeys(ctx, copiedKeys)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMusicNotFound
		}
		return err
	}
	if err := s.deleteMusicAssetKeys(ctx, []string{tempKey}); err != nil {
		return err
	}
	if req.ID > 0 && oldAudioKey != nil {
		old := strings.TrimSpace(*oldAudioKey)
		if old != "" && old != normalizedAudio && replaceableFormalMusicAudioKey(req.ID, old) {
			_ = s.deleteMusicAssetKeys(ctx, []string{old})
		}
	}
	return nil
}

func (s *musicService) DeleteMusic(id uint) error {
	err := s.repo.DeleteMusic(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMusicNotFound
	}
	return err
}

func (s *musicService) SaveArtist(ctx context.Context, userID uint, req dto.MusicArtistSaveReq) (*dto.MusicArtistResp, error) {
	var oldAvatarKey *string
	if req.ID > 0 {
		artists, err := s.repo.FindArtists([]uint{req.ID})
		if err != nil {
			return nil, err
		}
		if len(artists) == 0 {
			return nil, ErrMusicArtistNotFound
		}
		oldAvatarKey = artists[0].AvatarKey
	}

	var copiedKeys []string
	var tempKey string
	var normalizedAvatar string

	saved, err := s.repo.SaveArtist(musicrepo.MusicArtistSaveData{
		Artist: model.MusicArtist{
			Base:        model.Base{ID: req.ID},
			Name:        req.Name,
			NameZh:      req.NameZh,
			AvatarKey:   req.AvatarKey,
			Description: req.Description,
		},
		PrepareArtist: func(artist model.MusicArtist) (model.MusicArtist, error) {
			if req.AvatarKey == nil || strings.TrimSpace(*req.AvatarKey) == "" {
				return artist, nil
			}
			result, normalizeErr := normalizeMusicArtistAvatarKey(ctx, s.store, userID, artist.ID, *req.AvatarKey, oldAvatarKey)
			if normalizeErr != nil {
				return model.MusicArtist{}, normalizeErr
			}
			copiedKeys = result.CopiedKeys
			tempKey = result.TempKey
			normalizedAvatar = result.Key
			artist.AvatarKey = &result.Key
			return artist, nil
		},
	})
	if err != nil {
		_ = s.deleteMusicAssetKeys(ctx, copiedKeys)
		return nil, err
	}
	if saved == nil {
		return nil, ErrMusicArtistNotFound
	}
	if err := s.deleteMusicAssetKeys(ctx, []string{tempKey}); err != nil {
		return nil, err
	}
	if req.ID > 0 && oldAvatarKey != nil {
		old := strings.TrimSpace(*oldAvatarKey)
		if old != "" && old != normalizedAvatar && replaceableFormalMusicArtistAvatarKey(req.ID, old) {
			_ = s.deleteMusicAssetKeys(ctx, []string{old})
		}
	}
	resp := s.artistToDTO(saved)
	return &resp, nil
}

func (s *musicService) DeleteArtist(id uint) error {
	err := s.repo.DeleteArtist(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMusicArtistNotFound
	}
	return err
}

func (s *musicService) SaveAlbum(ctx context.Context, userID uint, req dto.MusicAlbumSaveReq) (*dto.MusicAlbumResp, error) {
	if req.ArtistID != nil {
		artists, err := s.repo.FindArtists([]uint{*req.ArtistID})
		if err != nil {
			return nil, err
		}
		if len(artists) == 0 {
			return nil, ErrMusicArtistNotFound
		}
	}

	releaseDate, err := parseOptionalDate(req.ReleaseDate)
	if err != nil {
		return nil, err
	}

	var oldCoverKey *string
	if req.ID > 0 {
		existing, findErr := s.repo.FindAlbum(req.ID)
		if findErr != nil {
			return nil, findErr
		}
		if existing == nil {
			return nil, ErrMusicAlbumNotFound
		}
		oldCoverKey = existing.CoverKey
	}

	var copiedKeys []string
	var tempKey string
	var normalizedCover string

	saved, err := s.repo.SaveAlbum(musicrepo.MusicAlbumSaveData{
		Album: model.MusicAlbum{
			Base:        model.Base{ID: req.ID},
			Name:        req.Name,
			ArtistID:    req.ArtistID,
			CoverKey:    req.CoverKey,
			ReleaseDate: releaseDate,
			Description: req.Description,
		},
		PrepareAlbum: func(album model.MusicAlbum) (model.MusicAlbum, error) {
			if req.CoverKey == nil || strings.TrimSpace(*req.CoverKey) == "" {
				return album, nil
			}
			result, normalizeErr := normalizeMusicAlbumCoverKey(ctx, s.store, userID, album.ID, *req.CoverKey, oldCoverKey)
			if normalizeErr != nil {
				return model.MusicAlbum{}, normalizeErr
			}
			copiedKeys = result.CopiedKeys
			tempKey = result.TempKey
			normalizedCover = result.Key
			album.CoverKey = &result.Key
			return album, nil
		},
	})
	if err != nil {
		_ = s.deleteMusicAssetKeys(ctx, copiedKeys)
		return nil, err
	}
	if saved == nil {
		return nil, ErrMusicAlbumNotFound
	}
	if err := s.deleteMusicAssetKeys(ctx, []string{tempKey}); err != nil {
		return nil, err
	}
	if req.ID > 0 && oldCoverKey != nil {
		old := strings.TrimSpace(*oldCoverKey)
		if old != "" && old != normalizedCover && replaceableFormalMusicAlbumCoverKey(req.ID, old) {
			_ = s.deleteMusicAssetKeys(ctx, []string{old})
		}
	}

	artistsByID := make(map[uint]model.MusicArtist)
	if saved.ArtistID != nil {
		var loadErr error
		artistsByID, loadErr = s.artistsByID([]uint{*saved.ArtistID})
		if loadErr != nil {
			return nil, loadErr
		}
	}
	return s.albumToDTO(saved, artistsByID), nil
}

func (s *musicService) DeleteAlbum(id uint) error {
	err := s.repo.DeleteAlbum(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMusicAlbumNotFound
	}
	return err
}

func (s *musicService) deleteMusicAssetKeys(ctx context.Context, keys []string) error {
	if s.store == nil {
		return nil
	}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if err := s.store.DeleteObject(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func uniqueArtistIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func parseOptionalDate(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.DateOnly, trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (s *musicService) musicListToDTO(rows []model.Music) (*dto.MusicListResp, error) {
	if len(rows) == 0 {
		return &dto.MusicListResp{List: []dto.MusicItemResp{}}, nil
	}

	ids := make([]uint, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}

	relations, err := s.repo.MusicArtistRelations(ids)
	if err != nil {
		return nil, err
	}

	albums, artistsByID, err := s.loadAlbumContext(collectMusicAlbumIDs(rows), relations)
	if err != nil {
		return nil, err
	}

	items := make([]dto.MusicItemResp, 0, len(rows))
	for i := range rows {
		items = append(items, s.musicItemToDTO(&rows[i], relations, albums, artistsByID))
	}
	return &dto.MusicListResp{List: items}, nil
}

func collectMusicAlbumIDs(rows []model.Music) []uint {
	albumIDs := make([]uint, 0)
	seen := make(map[uint]struct{})
	for i := range rows {
		if rows[i].AlbumID == nil {
			continue
		}
		if _, ok := seen[*rows[i].AlbumID]; ok {
			continue
		}
		seen[*rows[i].AlbumID] = struct{}{}
		albumIDs = append(albumIDs, *rows[i].AlbumID)
	}
	return albumIDs
}

func (s *musicService) loadAlbumContext(
	albumIDs []uint,
	relations map[uint][]model.MusicArtist,
) (map[uint]*model.MusicAlbum, map[uint]model.MusicArtist, error) {
	albums := make(map[uint]*model.MusicAlbum, len(albumIDs))
	artistsByID := make(map[uint]model.MusicArtist)
	for _, artists := range relations {
		for _, artist := range artists {
			artistsByID[artist.ID] = artist
		}
	}

	missingArtistIDs := make([]uint, 0)
	for _, albumID := range albumIDs {
		album, err := s.repo.FindAlbum(albumID)
		if err != nil {
			return nil, nil, err
		}
		if album == nil {
			continue
		}
		albums[albumID] = album
		if album.ArtistID == nil {
			continue
		}
		if _, ok := artistsByID[*album.ArtistID]; ok {
			continue
		}
		missingArtistIDs = append(missingArtistIDs, *album.ArtistID)
	}

	if len(missingArtistIDs) > 0 {
		found, err := s.repo.FindArtists(missingArtistIDs)
		if err != nil {
			return nil, nil, err
		}
		for _, artist := range found {
			artistsByID[artist.ID] = artist
		}
	}

	return albums, artistsByID, nil
}

func (s *musicService) artistsByID(ids []uint) (map[uint]model.MusicArtist, error) {
	result := make(map[uint]model.MusicArtist, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	rows, err := s.repo.FindArtists(ids)
	if err != nil {
		return nil, err
	}
	for _, artist := range rows {
		result[artist.ID] = artist
	}
	return result, nil
}

func (s *musicService) artistToDTO(artist *model.MusicArtist) dto.MusicArtistResp {
	return dto.MusicArtistResp{
		ID:          artist.ID,
		Name:        artist.Name,
		NameZh:      artist.NameZh,
		DisplayName: ArtistDisplayName(artist.Name, artist.NameZh),
		AvatarURL:   storage.ResolvePtrURL(s.store, artist.AvatarKey),
		Description: artist.Description,
	}
}

func (s *musicService) albumToDTO(album *model.MusicAlbum, artistsByID map[uint]model.MusicArtist) *dto.MusicAlbumResp {
	if album == nil {
		return nil
	}

	resp := &dto.MusicAlbumResp{
		ID:          album.ID,
		Name:        album.Name,
		CoverURL:    storage.ResolvePtrURL(s.store, album.CoverKey),
		Description: album.Description,
	}
	if album.ReleaseDate != nil {
		date := album.ReleaseDate.Format(time.DateOnly)
		resp.ReleaseDate = &date
	}
	if album.ArtistID != nil {
		if artist, ok := artistsByID[*album.ArtistID]; ok {
			artistDTO := s.artistToDTO(&artist)
			resp.Artist = &artistDTO
		}
	}
	return resp
}

func (s *musicService) musicItemToDTO(
	item *model.Music,
	relations map[uint][]model.MusicArtist,
	albums map[uint]*model.MusicAlbum,
	artistsByID map[uint]model.MusicArtist,
) dto.MusicItemResp {
	artistRows := relations[item.ID]
	artists := make([]dto.MusicArtistResp, 0, len(artistRows))
	for i := range artistRows {
		artists = append(artists, s.artistToDTO(&artistRows[i]))
	}

	var album *dto.MusicAlbumResp
	coverURL := storage.ResolvePtrURL(s.store, item.CoverImgUrl)
	if item.AlbumID != nil {
		if alb, ok := albums[*item.AlbumID]; ok {
			album = s.albumToDTO(alb, artistsByID)
			if album != nil && album.CoverURL != nil {
				coverURL = album.CoverURL
			}
		}
	}

	audioKey := item.AudioKey
	if audioKey == nil {
		audioKey = item.URL
	}

	return dto.MusicItemResp{
		ID:                item.ID,
		Name:              item.Name,
		ArtistDisplayName: item.ArtistDisplayName,
		Artists:           artists,
		Album:             album,
		AlbumTrackNo:      item.AlbumTrackNo,
		AudioURL:          storage.ResolvePtrURL(s.store, audioKey),
		CoverURL:          coverURL,
		Duration:          item.Duration,
		IsPublic:          item.IsPublic,
		Seq:               item.Seq,
	}
}
