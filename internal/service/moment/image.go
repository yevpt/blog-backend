package moment

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/pkg/imageutil"
	"github.com/vpt/blog-backend/pkg/storage"
)

const (
	momentImageObjectPrefix   = "moments/"
	maxMomentImageCount       = 9
	maxMomentImageOriginal    = 1024 * 1024
	maxMomentImageStoredBytes = 500 * 1024
)

func (s *momentService) prepareMomentImages(ctx context.Context, moment model.Moment, req dto.MomentSaveReq, uploaded *[]string) ([]model.Media, error) {
	if len(req.ImageURLs) == 0 && len(req.ImageFiles) == 0 {
		return nil, nil
	}
	if validMomentImageURLCount(req.ImageURLs)+len(req.ImageFiles) > maxMomentImageCount {
		return nil, ErrMomentImageInvalid
	}
	if moment.ID == 0 || moment.UserID == 0 {
		return nil, ErrMomentInvalid
	}

	store, err := s.momentObjectStore()
	if err != nil {
		return nil, err
	}

	images := make([]model.Media, 0, len(req.ImageURLs)+len(req.ImageFiles))
	if len(req.ImageOrder) > 0 {
		return s.prepareOrderedMomentImages(ctx, store, moment, req, uploaded)
	}

	for _, rawURL := range req.ImageURLs {
		image, err := existingMomentImage(ctx, store, rawURL, uint(len(images)+1))
		if err != nil {
			return nil, err
		}
		if image.URL != "" {
			images = append(images, image)
		}
	}
	for _, file := range req.ImageFiles {
		image, err := uploadMomentImage(ctx, store, moment, file, uint(len(images)+1), uploaded)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, nil
}

func (s *momentService) prepareOrderedMomentImages(
	ctx context.Context,
	store storage.ObjectStore,
	moment model.Moment,
	req dto.MomentSaveReq,
	uploaded *[]string,
) ([]model.Media, error) {
	expected := validMomentImageURLCount(req.ImageURLs) + len(req.ImageFiles)
	if len(req.ImageOrder) != expected {
		return nil, ErrMomentImageInvalid
	}

	seenURLs := map[int]struct{}{}
	seenFiles := map[int]struct{}{}
	images := make([]model.Media, 0, expected)
	for _, token := range req.ImageOrder {
		kind, index, err := parseMomentImageOrderToken(token)
		if err != nil {
			return nil, err
		}

		switch kind {
		case "url":
			if index < 0 || index >= len(req.ImageURLs) {
				return nil, ErrMomentImageInvalid
			}
			if _, exists := seenURLs[index]; exists {
				return nil, ErrMomentImageInvalid
			}
			seenURLs[index] = struct{}{}
			image, err := existingMomentImage(ctx, store, req.ImageURLs[index], uint(len(images)+1))
			if err != nil {
				return nil, err
			}
			if image.URL == "" {
				return nil, ErrMomentImageInvalid
			}
			images = append(images, image)
		case "file":
			if index < 0 || index >= len(req.ImageFiles) {
				return nil, ErrMomentImageInvalid
			}
			if _, exists := seenFiles[index]; exists {
				return nil, ErrMomentImageInvalid
			}
			seenFiles[index] = struct{}{}
			image, err := uploadMomentImage(ctx, store, moment, req.ImageFiles[index], uint(len(images)+1), uploaded)
			if err != nil {
				return nil, err
			}
			images = append(images, image)
		default:
			return nil, ErrMomentImageInvalid
		}
	}
	if len(seenURLs) != validMomentImageURLCount(req.ImageURLs) || len(seenFiles) != len(req.ImageFiles) {
		return nil, ErrMomentImageInvalid
	}
	return images, nil
}

func parseMomentImageOrderToken(token string) (string, int, error) {
	kind, rawIndex, ok := strings.Cut(strings.TrimSpace(token), ":")
	if !ok {
		return "", 0, ErrMomentImageInvalid
	}
	index, err := strconv.Atoi(rawIndex)
	if err != nil || index < 0 {
		return "", 0, ErrMomentImageInvalid
	}
	return kind, index, nil
}

func validMomentImageURLCount(urls []string) int {
	count := 0
	for _, value := range urls {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func (s *momentService) momentObjectStore() (storage.ObjectStore, error) {
	store, ok := s.objectURLResolver.(storage.ObjectStore)
	if !ok || store == nil {
		return nil, errors.New("对象存储不支持碎语图片上传")
	}
	return store, nil
}

func existingMomentImage(ctx context.Context, store storage.ObjectStore, rawURL string, seq uint) (model.Media, error) {
	key := momentImageObjectKey(rawURL)
	if key == "" {
		if strings.TrimSpace(rawURL) == "" {
			return model.Media{}, nil
		}
		return model.Media{}, ErrMomentImageInvalid
	}
	exists, err := store.ObjectExists(ctx, key)
	if err != nil {
		return model.Media{}, err
	}
	if !exists {
		return model.Media{}, ErrMomentImageNotFound
	}

	name := path.Base(key)
	return model.Media{
		Name:     name,
		FileType: fileTypeFromName(name),
		URL:      key,
		Seq:      seq,
	}, nil
}

func uploadMomentImage(
	ctx context.Context,
	store storage.ObjectStore,
	moment model.Moment,
	file dto.MomentImageFileReq,
	seq uint,
	uploaded *[]string,
) (model.Media, error) {
	if len(file.Data) == 0 || !strings.HasPrefix(strings.ToLower(file.ContentType), "image/") {
		return model.Media{}, ErrMomentImageInvalid
	}
	processed, err := processMomentImageFile(file)
	if err != nil {
		return model.Media{}, err
	}

	objectName := uploadedMomentImageObjectName(moment, processed)
	exists, err := store.ObjectExists(ctx, objectName)
	if err != nil {
		return model.Media{}, err
	}
	if !exists {
		if err := store.PutObject(ctx, objectName, processed.Data, processed.ContentType); err != nil {
			return model.Media{}, err
		}
		*uploaded = append(*uploaded, objectName)
	}

	return model.Media{
		Name:     file.Name,
		FileType: strings.TrimPrefix(processed.Ext, "."),
		URL:      objectName,
		Size:     uint(len(processed.Data)),
		Seq:      seq,
	}, nil
}

func (s *momentService) deleteUploadedMomentImages(ctx context.Context, uploaded []string) error {
	if len(uploaded) == 0 {
		return nil
	}
	store, err := s.momentObjectStore()
	if err != nil {
		return err
	}

	var cleanupErr error
	for _, objectName := range uploaded {
		if err := store.DeleteObject(ctx, objectName); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func (s *momentService) deleteRemovedMomentImages(ctx context.Context, urls []string) error {
	if len(urls) == 0 {
		return nil
	}
	store, err := s.momentObjectStore()
	if err != nil {
		return err
	}

	var cleanupErr error
	seen := map[string]struct{}{}
	for _, rawURL := range urls {
		key := momentImageObjectKey(rawURL)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if err := store.DeleteObject(ctx, key); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

type processedMomentImage struct {
	Data        []byte
	ContentType string
	Ext         string
}

func processMomentImageFile(file dto.MomentImageFileReq) (processedMomentImage, error) {
	if len(file.Data) > maxMomentImageOriginal {
		return processedMomentImage{}, ErrMomentImageTooLarge
	}

	format := imageFormatFromFile(file)
	opts := imageutil.Options{Format: format}
	if len(file.Data) > maxMomentImageStoredBytes {
		opts.Format = imageutil.FormatJPEG
		opts.MaxBytes = maxMomentImageStoredBytes
		opts.JPEGQuality = 85
		opts.MinJPEGQuality = 35
	}
	result, err := imageutil.Process(bytes.NewReader(file.Data), opts)
	if errors.Is(err, imageutil.ErrInvalidImage) ||
		errors.Is(err, imageutil.ErrUnsupportedFormat) ||
		errors.Is(err, imageutil.ErrImageTooLarge) {
		return processedMomentImage{}, ErrMomentImageInvalid
	}
	if err != nil {
		return processedMomentImage{}, err
	}
	if len(result.Bytes) > maxMomentImageStoredBytes {
		result, err = imageutil.Process(bytes.NewReader(file.Data), imageutil.Options{
			Format:         imageutil.FormatJPEG,
			MaxBytes:       maxMomentImageStoredBytes,
			JPEGQuality:    85,
			MinJPEGQuality: 35,
		})
		if errors.Is(err, imageutil.ErrImageTooLarge) {
			return processedMomentImage{}, ErrMomentImageInvalid
		}
		if err != nil {
			return processedMomentImage{}, err
		}
	}

	return processedMomentImage{Data: result.Bytes, ContentType: result.ContentType, Ext: result.Ext}, nil
}

func uploadedMomentImageObjectName(moment model.Moment, image processedMomentImage) string {
	return fmt.Sprintf(
		"%s%d/%d/%s%s",
		momentImageObjectPrefix,
		moment.UserID,
		moment.ID,
		fileMD5(image.Data),
		image.Ext,
	)
}

func fileMD5(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func momentImageObjectKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	rawPath := value
	if storage.IsAbsoluteURL(value) {
		parsed, err := url.Parse(value)
		if err != nil {
			return ""
		}
		rawPath = parsed.Path
	}
	rawPath, _, _ = strings.Cut(rawPath, "?")
	rawPath, _, _ = strings.Cut(rawPath, "#")

	key := strings.TrimLeft(strings.TrimSpace(rawPath), "/")
	index := strings.Index(key, momentImageObjectPrefix)
	if index < 0 {
		return ""
	}
	return key[index:]
}

func fileExt(file dto.MomentImageFileReq) string {
	if ext := strings.ToLower(filepath.Ext(file.Name)); ext != "" {
		return ext
	}
	exts, err := mime.ExtensionsByType(file.ContentType)
	if err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ".img"
}

func imageFormatFromFile(file dto.MomentImageFileReq) imageutil.Format {
	ext := strings.ToLower(strings.TrimPrefix(fileExt(file), "."))
	if ext == "png" {
		return imageutil.FormatPNG
	}
	return imageutil.FormatJPEG
}

func fileTypeFromName(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
}
