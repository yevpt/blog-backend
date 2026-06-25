package music

import (
	"context"
	"errors"
	"fmt"

	"github.com/vpt/blog-backend/internal/dto"
	uploadservice "github.com/vpt/blog-backend/internal/service/upload"
	"github.com/vpt/blog-backend/pkg/audiofile"
	"github.com/vpt/blog-backend/pkg/imagefile"
)

const MaxMusicAudioBytes = 50 * 1024 * 1024

var ErrMusicUploadInvalid = errors.New("音乐资源无效")

type MusicAudioUploadInput struct {
	UserID uint
	Name   string
	Data   []byte
}

type MusicImageUploadInput struct {
	UserID uint
	Name   string
	Data   []byte
}

func (s *musicService) UploadAudio(ctx context.Context, input MusicAudioUploadInput) (*dto.MusicUploadResp, error) {
	result, err := audiofile.Validate(input.Name, input.Data, MaxMusicAudioBytes)
	if err != nil {
		if errors.Is(err, audiofile.ErrInvalidAudio) {
			return nil, ErrMusicUploadInvalid
		}
		return nil, err
	}
	key := fmt.Sprintf("temp/music/%d/audio/%s%s", input.UserID, result.SHA256, result.Ext)
	return s.putMusicUpload(ctx, key, result.Data, result.ContentType, result.SHA256, result.Size, result.ContentType)
}

func (s *musicService) UploadAlbumCover(ctx context.Context, input MusicImageUploadInput) (*dto.MusicUploadResp, error) {
	return s.uploadMusicImage(ctx, input, "album-cover")
}

func (s *musicService) UploadArtistAvatar(ctx context.Context, input MusicImageUploadInput) (*dto.MusicUploadResp, error) {
	return s.uploadMusicImage(ctx, input, "artist-avatar")
}

func (s *musicService) uploadMusicImage(ctx context.Context, input MusicImageUploadInput, dir string) (*dto.MusicUploadResp, error) {
	result, err := imagefile.Validate(input.Name, input.Data, uploadservice.MaxTempImageBytes)
	if err != nil {
		return nil, ErrMusicUploadInvalid
	}
	key := fmt.Sprintf("temp/music/%d/%s/%s%s", input.UserID, dir, result.MD5, result.Ext)
	return s.putMusicUpload(ctx, key, result.Data, result.ContentType, result.MD5, uint64(len(result.Data)), result.ContentType)
}

func (s *musicService) putMusicUpload(
	ctx context.Context,
	key string,
	data []byte,
	contentType string,
	hash string,
	size uint64,
	mime string,
) (*dto.MusicUploadResp, error) {
	if s.store == nil {
		return nil, ErrMusicUploadInvalid
	}

	exists, err := s.store.ObjectExists(ctx, key)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := s.store.PutObject(ctx, key, data, contentType); err != nil {
			return nil, err
		}
	}

	url, err := s.store.ObjectURL(ctx, key)
	if err != nil {
		return nil, err
	}
	return &dto.MusicUploadResp{
		Key:  key,
		URL:  url,
		Size: size,
		Mime: mime,
		Hash: hash,
	}, nil
}
