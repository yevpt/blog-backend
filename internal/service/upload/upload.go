package upload

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/pkg/imagefile"
	"github.com/vpt/blog-backend/pkg/storage"
)

const MaxTempImageBytes = 10 * 1024 * 1024

var (
	ErrUploadInvalid     = errors.New("上传图片无效")
	ErrUploadTooLarge    = errors.New("图片不能超过 10MB")
	ErrUploadDirInvalid  = errors.New("上传目录无效")
	ErrUploadUnavailable = errors.New("对象存储不可用")
)

type Service interface {
	UploadTempImage(ctx context.Context, input TempImageInput) (*dto.TempUploadResp, error)
}

type TempImageInput struct {
	UserID uint
	Dir    string
	Name   string
	Data   []byte
}

type service struct {
	store storage.ObjectStore
}

func NewService(store storage.ObjectStore) Service {
	return &service{store: store}
}

func (s *service) UploadTempImage(ctx context.Context, input TempImageInput) (*dto.TempUploadResp, error) {
	if s == nil || s.store == nil {
		return nil, ErrUploadUnavailable
	}

	dir, err := normalizeTempDir(input.Dir)
	if err != nil {
		return nil, err
	}

	result, err := imagefile.Validate(input.Name, input.Data, MaxTempImageBytes)
	if err != nil {
		return nil, mapValidateErr(err)
	}

	key := fmt.Sprintf("temp/articles/%d/%s/%s%s", input.UserID, dir, result.MD5, result.Ext)
	exists, err := s.store.ObjectExists(ctx, key)
	if err != nil {
		return nil, ErrUploadUnavailable
	}
	if !exists {
		if err := s.store.PutObject(ctx, key, result.Data, result.ContentType); err != nil {
			return nil, ErrUploadUnavailable
		}
	}
	url, err := s.store.ObjectURL(ctx, key)
	if err != nil {
		return nil, ErrUploadUnavailable
	}

	return &dto.TempUploadResp{
		Key: key,
		URL: url,
	}, nil
}

func normalizeTempDir(value string) (string, error) {
	dir := strings.TrimSpace(value)
	switch dir {
	case "images", "covers":
		return dir, nil
	default:
		return "", ErrUploadDirInvalid
	}
}

func mapValidateErr(err error) error {
	switch {
	case errors.Is(err, imagefile.ErrImageTooLarge):
		return ErrUploadTooLarge
	case errors.Is(err, imagefile.ErrInvalidImage):
		return ErrUploadInvalid
	default:
		return ErrUploadInvalid
	}
}
