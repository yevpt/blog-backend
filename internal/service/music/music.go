package music

import (
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	musicrepo "github.com/vpt/blog-backend/internal/repository/music"
	"github.com/vpt/blog-backend/pkg/storage"
)

// MusicService 音乐业务接口。
type MusicService interface {
	List() (*dto.MusicListResp, error)
}

type musicService struct {
	repo     musicrepo.MusicRepository
	resolver storage.ObjectURLResolver
}

// NewMusicService 创建音乐业务服务实例。
func NewMusicService(repo musicrepo.MusicRepository, resolver storage.ObjectURLResolver) MusicService {
	return &musicService{repo: repo, resolver: resolver}
}

func (s *musicService) List() (*dto.MusicListResp, error) {
	rows, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	items := make([]dto.MusicItemResp, 0, len(rows))
	for i := range rows {
		items = append(items, s.musicToDTO(&rows[i]))
	}
	return &dto.MusicListResp{List: items}, nil
}

func (s *musicService) musicToDTO(item *model.Music) dto.MusicItemResp {
	return dto.MusicItemResp{
		ID:          item.ID,
		Name:        item.Name,
		Singer:      item.Singer,
		Album:       item.Album,
		URL:         storage.ResolvePtrURL(s.resolver, item.URL),
		CoverImgUrl: storage.ResolvePtrURL(s.resolver, item.CoverImgUrl),
		Duration:    item.Duration,
		Seq:         item.Seq,
	}
}
