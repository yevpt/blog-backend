package music

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	ErrMusicAssetInvalid  = errors.New("音乐资源无效")
	ErrMusicAssetNotFound = errors.New("音乐资源不存在")
)

type musicAssetNormalizeResult struct {
	Key        string
	TempKey    string
	CopiedKeys []string
}

type musicAssetNormalizer struct {
	ctx    context.Context
	store  storage.ObjectStore
	userID uint
}

func normalizeMusicAudioKey(
	ctx context.Context,
	store storage.ObjectStore,
	userID, musicID uint,
	rawKey string,
	previousKey *string,
) (*musicAssetNormalizeResult, error) {
	n := &musicAssetNormalizer{ctx: ctx, store: store, userID: userID}
	key, err := n.objectKey(rawKey)
	if err != nil {
		return nil, err
	}
	if n.sameAsPrevious(key, previousKey) {
		return &musicAssetNormalizeResult{Key: key}, nil
	}

	tempPrefix := fmt.Sprintf("temp/music/%d/audio/", userID)
	formalPrefix := fmt.Sprintf("music/audio/%d/", musicID)
	switch {
	case strings.HasPrefix(key, tempPrefix):
		return n.promote(key, formalPrefix+path.Base(key))
	case strings.HasPrefix(key, formalPrefix):
		return n.confirmFormal(key)
	case strings.HasPrefix(key, "temp/music/"):
		return nil, fmt.Errorf("%w：%s", ErrMusicAssetInvalid, key)
	case strings.HasPrefix(key, "music/audio/"):
		return nil, fmt.Errorf("%w：%s", ErrMusicAssetInvalid, key)
	default:
		return n.allowExisting(key)
	}
}

func normalizeMusicAlbumCoverKey(
	ctx context.Context,
	store storage.ObjectStore,
	userID, albumID uint,
	rawKey string,
	previousKey *string,
) (*musicAssetNormalizeResult, error) {
	n := &musicAssetNormalizer{ctx: ctx, store: store, userID: userID}
	key, err := n.objectKey(rawKey)
	if err != nil {
		return nil, err
	}
	if n.sameAsPrevious(key, previousKey) {
		return &musicAssetNormalizeResult{Key: key}, nil
	}

	tempPrefix := fmt.Sprintf("temp/music/%d/album-cover/", userID)
	formalPrefix := fmt.Sprintf("music/albums/%d/cover/", albumID)
	switch {
	case strings.HasPrefix(key, tempPrefix):
		return n.promote(key, formalPrefix+path.Base(key))
	case strings.HasPrefix(key, formalPrefix):
		return n.confirmFormal(key)
	case strings.HasPrefix(key, "temp/music/"):
		return nil, fmt.Errorf("%w：%s", ErrMusicAssetInvalid, key)
	case strings.HasPrefix(key, "music/albums/"):
		return nil, fmt.Errorf("%w：%s", ErrMusicAssetInvalid, key)
	default:
		return n.allowExisting(key)
	}
}

func normalizeMusicArtistAvatarKey(
	ctx context.Context,
	store storage.ObjectStore,
	userID, artistID uint,
	rawKey string,
	previousKey *string,
) (*musicAssetNormalizeResult, error) {
	n := &musicAssetNormalizer{ctx: ctx, store: store, userID: userID}
	key, err := n.objectKey(rawKey)
	if err != nil {
		return nil, err
	}
	if n.sameAsPrevious(key, previousKey) {
		return &musicAssetNormalizeResult{Key: key}, nil
	}

	tempPrefix := fmt.Sprintf("temp/music/%d/artist-avatar/", userID)
	formalPrefix := fmt.Sprintf("music/artists/%d/avatar/", artistID)
	switch {
	case strings.HasPrefix(key, tempPrefix):
		return n.promote(key, formalPrefix+path.Base(key))
	case strings.HasPrefix(key, formalPrefix):
		return n.confirmFormal(key)
	case strings.HasPrefix(key, "temp/music/"):
		return nil, fmt.Errorf("%w：%s", ErrMusicAssetInvalid, key)
	case strings.HasPrefix(key, "music/artists/"):
		return nil, fmt.Errorf("%w：%s", ErrMusicAssetInvalid, key)
	default:
		return n.allowExisting(key)
	}
}

func (n *musicAssetNormalizer) objectKey(rawValue string) (string, error) {
	if n.store == nil {
		return "", ErrMusicAssetInvalid
	}
	key, err := n.store.ObjectKey(rawValue)
	if err == nil {
		return key, nil
	}
	if errors.Is(err, storage.ErrExternalObjectURL) {
		return "", fmt.Errorf("%w：%s", ErrMusicAssetInvalid, rawValue)
	}
	trimmed := strings.TrimSpace(rawValue)
	if trimmed != "" && !storage.IsAbsoluteURL(trimmed) {
		return trimmed, nil
	}
	return "", fmt.Errorf("%w：%s", ErrMusicAssetInvalid, rawValue)
}

func (n *musicAssetNormalizer) sameAsPrevious(key string, previousKey *string) bool {
	if previousKey == nil {
		return false
	}
	previous := strings.TrimSpace(*previousKey)
	return previous != "" && key == previous
}

func (n *musicAssetNormalizer) promote(source, target string) (*musicAssetNormalizeResult, error) {
	exists, err := n.store.ObjectExists(n.ctx, target)
	if err != nil {
		return nil, err
	}
	result := &musicAssetNormalizeResult{TempKey: source}
	if !exists {
		if err := n.store.CopyObject(n.ctx, source, target); err != nil {
			return nil, err
		}
		result.CopiedKeys = []string{target}
	}
	if err := n.ensureExists(target); err != nil {
		return nil, err
	}
	result.Key = target
	return result, nil
}

func (n *musicAssetNormalizer) confirmFormal(key string) (*musicAssetNormalizeResult, error) {
	if err := n.ensureExists(key); err != nil {
		return nil, err
	}
	return &musicAssetNormalizeResult{Key: key}, nil
}

func (n *musicAssetNormalizer) allowExisting(key string) (*musicAssetNormalizeResult, error) {
	if err := n.ensureExists(key); err != nil {
		return nil, err
	}
	return &musicAssetNormalizeResult{Key: key}, nil
}

func (n *musicAssetNormalizer) ensureExists(key string) error {
	exists, err := n.store.ObjectExists(n.ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w：%s", ErrMusicAssetNotFound, key)
	}
	return nil
}

func sortedAssetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func replaceableFormalMusicAudioKey(musicID uint, key string) bool {
	prefix := fmt.Sprintf("music/audio/%d/", musicID)
	return strings.HasPrefix(strings.TrimSpace(key), prefix)
}

func replaceableFormalMusicAlbumCoverKey(albumID uint, key string) bool {
	prefix := fmt.Sprintf("music/albums/%d/cover/", albumID)
	return strings.HasPrefix(strings.TrimSpace(key), prefix)
}

func replaceableFormalMusicArtistAvatarKey(artistID uint, key string) bool {
	prefix := fmt.Sprintf("music/artists/%d/avatar/", artistID)
	return strings.HasPrefix(strings.TrimSpace(key), prefix)
}
