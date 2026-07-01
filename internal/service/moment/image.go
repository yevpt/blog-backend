package moment

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
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
	"github.com/vpt/blog-backend/pkg/imagefile"
	"github.com/vpt/blog-backend/pkg/imageutil"
	"github.com/vpt/blog-backend/pkg/storage"
)

const (
	momentImageObjectPrefix   = "moments/"
	momentStagingObjectPrefix = "moderation/staging/moments/"
	maxMomentImageCount       = 9
	maxMomentImageOriginal    = 3 * 1024 * 1024
	maxMomentGifOriginal      = 300 * 1024
	maxMomentImageStoredBytes = 500 * 1024
)

func (s *momentService) prepareModerationMomentImages(
	ctx context.Context,
	authorID uint,
	req dto.MomentSaveReq,
	uploaded *[]string,
) ([]string, error) {
	expected := validMomentImageURLCount(req.ImageURLs) + len(req.ImageFiles)
	if expected == 0 {
		return nil, nil
	}
	if expected > maxMomentImageCount || authorID == 0 {
		return nil, ErrMomentImageInvalid
	}
	store, err := s.momentObjectStore()
	if err != nil {
		return nil, err
	}
	appendURL := func(result []string, index int) ([]string, error) {
		if index < 0 || index >= len(req.ImageURLs) {
			return nil, ErrMomentImageInvalid
		}
		key := momentImageObjectKey(req.ImageURLs[index])
		if key == "" || !reusableModerationMomentImage(key, authorID, req.ID) {
			return nil, ErrMomentImageInvalid
		}
		exists, existsErr := store.ObjectExists(ctx, key)
		if existsErr != nil {
			return nil, existsErr
		}
		if !exists {
			return nil, ErrMomentImageNotFound
		}
		if momentStagingImageBelongsToUser(key, authorID) {
			target := fmt.Sprintf("%s%d/%s/%s", momentStagingObjectPrefix, authorID, momentUploadBatchID(req.IdempotencyKey), filepath.Base(key))
			if target != key {
				targetExists, targetErr := store.ObjectExists(ctx, target)
				if targetErr != nil {
					return nil, targetErr
				}
				if !targetExists {
					if copyErr := store.CopyObject(ctx, key, target); copyErr != nil {
						return nil, copyErr
					}
					*uploaded = append(*uploaded, target)
				}
				key = target
			}
		}
		return append(result, key), nil
	}
	appendFile := func(result []string, index int) ([]string, error) {
		if index < 0 || index >= len(req.ImageFiles) {
			return nil, ErrMomentImageInvalid
		}
		processed, processErr := processMomentImageFile(req.ImageFiles[index])
		if processErr != nil {
			return nil, processErr
		}
		key := fmt.Sprintf("%s%d/%s/%s%s", momentStagingObjectPrefix, authorID, momentUploadBatchID(req.IdempotencyKey), fileMD5(processed.Data), processed.Ext)
		exists, existsErr := store.ObjectExists(ctx, key)
		if existsErr != nil {
			return nil, existsErr
		}
		if !exists {
			if putErr := store.PutObject(ctx, key, processed.Data, processed.ContentType); putErr != nil {
				return nil, putErr
			}
			*uploaded = append(*uploaded, key)
		}
		return append(result, key), nil
	}

	result := make([]string, 0, expected)
	if len(req.ImageOrder) == 0 {
		for index := range req.ImageURLs {
			if strings.TrimSpace(req.ImageURLs[index]) == "" {
				continue
			}
			result, err = appendURL(result, index)
			if err != nil {
				return nil, err
			}
		}
		for index := range req.ImageFiles {
			result, err = appendFile(result, index)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	if len(req.ImageOrder) != expected {
		return nil, ErrMomentImageInvalid
	}
	seenURLs, seenFiles := map[int]struct{}{}, map[int]struct{}{}
	for _, token := range req.ImageOrder {
		kind, index, parseErr := parseMomentImageOrderToken(token)
		if parseErr != nil {
			return nil, parseErr
		}
		switch kind {
		case "url":
			if _, exists := seenURLs[index]; exists {
				return nil, ErrMomentImageInvalid
			}
			seenURLs[index] = struct{}{}
			result, err = appendURL(result, index)
		case "file":
			if _, exists := seenFiles[index]; exists {
				return nil, ErrMomentImageInvalid
			}
			seenFiles[index] = struct{}{}
			result, err = appendFile(result, index)
		default:
			return nil, ErrMomentImageInvalid
		}
		if err != nil {
			return nil, err
		}
	}
	if len(seenURLs) != validMomentImageURLCount(req.ImageURLs) || len(seenFiles) != len(req.ImageFiles) {
		return nil, ErrMomentImageInvalid
	}
	return result, nil
}

// momentUploadBatchID 为同一幂等请求生成稳定目录，避免重试产生不同暂存对象。
func momentUploadBatchID(idempotencyKey string) string {
	sum := sha256.Sum256([]byte(idempotencyKey))
	return hex.EncodeToString(sum[:8])
}

const (
	momentImageReadableFormatsText = "JPG、PNG、WebP 或 300KB 以内的 GIF"
	momentGifTooLargeMessage       = "GIF 图片过大，暂不支持压缩该格式，请上传 300KB 以内的 GIF。"
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
		image, err := existingMomentImage(ctx, store, rawURL, moment.UserID, moment.ID, uint(len(images)+1))
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
			image, err := existingMomentImage(ctx, store, req.ImageURLs[index], moment.UserID, moment.ID, uint(len(images)+1))
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

func existingMomentImage(ctx context.Context, store storage.ObjectStore, rawURL string, userID, momentID, seq uint) (model.Media, error) {
	key := momentImageObjectKey(rawURL)
	if key == "" {
		if strings.TrimSpace(rawURL) == "" {
			return model.Media{}, nil
		}
		return model.Media{}, ErrMomentImageInvalid
	}
	if !momentImageObjectBelongsToMoment(key, userID, momentID) {
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

func reusableModerationMomentImage(key string, userID uint, momentID *uint) bool {
	return momentID != nil && (momentImageObjectBelongsToMoment(key, userID, *momentID) ||
		momentStagingImageBelongsToUser(key, userID))
}

func momentStagingImageBelongsToUser(key string, userID uint) bool {
	prefix := fmt.Sprintf("%s%d/", momentStagingObjectPrefix, userID)
	remainder := strings.TrimPrefix(key, prefix)
	return remainder != key && remainder != "" && !strings.Contains(remainder, "..") && filepath.Base(remainder) != "."
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

func (s *momentService) deleteRemovedMomentImages(ctx context.Context, userID, momentID uint, urls []string) error {
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
		if key == "" || !momentImageObjectBelongsToMoment(key, userID, momentID) {
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
	if isMomentGifFile(file) {
		validated, err := imagefile.Validate(file.Name, file.Data, maxMomentGifOriginal)
		if errors.Is(err, imagefile.ErrImageTooManyPixels) {
			return processedMomentImage{}, newMomentImageInvalidError(imagefile.ErrImageTooManyPixels.Error())
		}
		if errors.Is(err, imagefile.ErrImageTooLarge) {
			return processedMomentImage{}, newMomentImageInvalidError(momentGifTooLargeMessage)
		}
		if err != nil || validated.ContentType != "image/gif" {
			return processedMomentImage{}, newMomentImageInvalidError(
				"图片无法读取，请确认文件未损坏，并尝试换一张 " + momentImageReadableFormatsText,
			)
		}
		return processedMomentImage{Data: validated.Data, ContentType: validated.ContentType, Ext: validated.Ext}, nil
	}

	if len(file.Data) > maxMomentImageOriginal {
		return processedMomentImage{}, ErrMomentImageTooLarge
	}

	stored, err := imagefile.PrepareForStorage(file.Name, file.Data, imagefile.PrepareOptions{
		MaxStoredBytes: maxMomentImageStoredBytes,
	})
	if errors.Is(err, imagefile.ErrImageTooManyPixels) {
		return processedMomentImage{}, newMomentImageInvalidError(imagefile.ErrImageTooManyPixels.Error())
	}
	if errors.Is(err, imagefile.ErrInvalidImage) {
		return processedMomentImage{}, newMomentImageInvalidError(
			"图片无法读取，请确认文件未损坏，并尝试换一张 " + momentImageReadableFormatsText,
		)
	}
	if errors.Is(err, imageutil.ErrInvalidImage) {
		return processedMomentImage{}, newMomentImageInvalidError(
			"图片无法读取，请确认文件未损坏，并尝试换一张 " + momentImageReadableFormatsText,
		)
	}
	if errors.Is(err, imageutil.ErrUnsupportedFormat) {
		return processedMomentImage{}, newMomentImageInvalidError(
			"图片格式不支持，请上传 " + momentImageReadableFormatsText,
		)
	}
	if errors.Is(err, imageutil.ErrImageTooLarge) {
		return processedMomentImage{}, newMomentImageInvalidError(
			"图片过大，压缩后仍超过 500KB，请换一张更小的图片",
		)
	}
	if err != nil {
		return processedMomentImage{}, err
	}

	return processedMomentImage{Data: stored.Data, ContentType: stored.ContentType, Ext: stored.Ext}, nil
}

type momentImageInvalidError struct {
	message string
}

func newMomentImageInvalidError(message string) error {
	return momentImageInvalidError{message: message}
}

func (err momentImageInvalidError) Error() string {
	return err.message
}

func (err momentImageInvalidError) Is(target error) bool {
	return target == ErrMomentImageInvalid
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

func momentImageObjectBelongsToMoment(key string, userID, momentID uint) bool {
	if userID == 0 || momentID == 0 || key == "" {
		return false
	}
	return strings.HasPrefix(key, fmt.Sprintf("%s%d/%d/", momentImageObjectPrefix, userID, momentID))
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
	for _, prefix := range []string{momentStagingObjectPrefix, momentImageObjectPrefix} {
		if index := strings.Index(key, prefix); index >= 0 {
			return key[index:]
		}
	}
	return ""
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

func isMomentGifFile(file dto.MomentImageFileReq) bool {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(file.ContentType, ";")[0]))
	return contentType == "image/gif" || strings.EqualFold(fileExt(file), ".gif")
}

func fileTypeFromName(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
}
