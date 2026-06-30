package moderationmedia_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	moderationmedia "github.com/vpt/blog-backend/internal/service/moderationmedia"
)

type publishStore struct {
	objects  map[string][]byte
	copies   [][2]string
	copyErr  error
	deletes  []string
	existsFn func(string) bool
}

func (s *publishStore) CopyObject(_ context.Context, source, target string) error {
	if s.copyErr != nil {
		return s.copyErr
	}
	data, ok := s.objects[source]
	if !ok {
		return errors.New("source missing")
	}
	s.objects[target] = append([]byte(nil), data...)
	s.copies = append(s.copies, [2]string{source, target})
	return nil
}
func (s *publishStore) ObjectExists(_ context.Context, key string) (bool, error) {
	if s.existsFn != nil {
		return s.existsFn(key), nil
	}
	_, ok := s.objects[key]
	return ok, nil
}
func (s *publishStore) DeleteObject(_ context.Context, key string) error {
	delete(s.objects, key)
	s.deletes = append(s.deletes, key)
	return nil
}

type publishRegistry struct {
	called    bool
	command   moderationrepo.PublishedImageCommand
	resultErr error
}

func (r *publishRegistry) ApplyPublishedImageKeys(_ context.Context, cmd moderationrepo.PublishedImageCommand) error {
	r.called = true
	r.command = cmd
	return r.resultErr
}

func stagingImage(seq uint, sha, key string) moderationrepo.RevisionImageRecord {
	return moderationrepo.RevisionImageRecord{
		ImageFingerprint: moderationrepo.ImageFingerprint{SHA256: sha, MD5: "md5" + sha, Size: 10},
		Seq:              seq, ObjectKey: key, MediaType: "image/jpeg",
	}
}

func formalImage(seq uint, sha, key string) moderationrepo.RevisionImageRecord {
	return moderationrepo.RevisionImageRecord{
		ImageFingerprint: moderationrepo.ImageFingerprint{SHA256: sha, MD5: "md5" + sha, Size: 10},
		Seq:              seq, ObjectKey: key, MediaType: "image/jpeg",
	}
}

func TestPublishCopiesStagingToFormalAndAppliesKeys(t *testing.T) {
	staging := "moderation/staging/moments/42/u1/sha.jpg"
	formal := "moments/42/8/sha.jpg"
	store := &publishStore{objects: map[string][]byte{staging: []byte("img")}}
	registry := &publishRegistry{}
	publisher := moderationmedia.NewPublisher(store, registry)

	result, err := publisher.Publish(context.Background(), moderationmedia.PublishCommand{
		ItemID: 10, RevisionID: 101, UserID: 42, MomentID: 8,
		Current: []moderationrepo.RevisionImageRecord{stagingImage(1, "sha", staging)},
	})

	require.NoError(t, err)
	require.True(t, registry.called)
	require.Len(t, result.Images, 1)
	assert.Equal(t, staging, result.Images[0].SourceKey)
	assert.Equal(t, formal, result.Images[0].PublicKey)
	require.Len(t, store.copies, 1)
	assert.Equal(t, [2]string{staging, formal}, store.copies[0])
	assert.Equal(t, []moderationrepo.PublishedImageKey{{Seq: 1, ObjectKey: formal}}, registry.command.ImageKeys)
	assert.Contains(t, store.deletes, staging)
}

func TestPublishKeepsAlreadyFormalImageWithoutCopy(t *testing.T) {
	formal := "moments/42/8/sha.jpg"
	store := &publishStore{objects: map[string][]byte{formal: []byte("img")}}
	registry := &publishRegistry{}
	publisher := moderationmedia.NewPublisher(store, registry)

	result, err := publisher.Publish(context.Background(), moderationmedia.PublishCommand{
		ItemID: 10, RevisionID: 102, UserID: 42, MomentID: 8,
		Current:  []moderationrepo.RevisionImageRecord{formalImage(1, "sha", formal)},
		Previous: []moderationrepo.RevisionImageRecord{formalImage(1, "sha", formal)},
	})

	require.NoError(t, err)
	require.True(t, registry.called)
	require.Len(t, result.Images, 1)
	assert.Equal(t, formal, result.Images[0].PublicKey)
	assert.Empty(t, store.copies)
	assert.NotContains(t, store.deletes, formal)
}

func TestPublishMovesRemovedImageToAudit(t *testing.T) {
	staging := "moderation/staging/moments/42/u2/new.jpg"
	formalNew := "moments/42/8/new.jpg"
	oldPublic := "moments/42/8/old.jpg"
	auditKey := "moderation/history/moments/10/old.jpg"
	store := &publishStore{objects: map[string][]byte{
		staging:   []byte("new"),
		oldPublic: []byte("old"),
	}}
	registry := &publishRegistry{}
	publisher := moderationmedia.NewPublisher(store, registry)

	result, err := publisher.Publish(context.Background(), moderationmedia.PublishCommand{
		ItemID: 10, RevisionID: 102, UserID: 42, MomentID: 8,
		Current:  []moderationrepo.RevisionImageRecord{stagingImage(1, "new", staging)},
		Previous: []moderationrepo.RevisionImageRecord{formalImage(1, "old", oldPublic)},
	})

	require.NoError(t, err)
	require.True(t, registry.called)
	require.Len(t, result.Images, 1)
	assert.Equal(t, formalNew, result.Images[0].PublicKey)
	require.Len(t, result.AuditMoves, 1)
	assert.Equal(t, auditKey, result.AuditMoves[oldPublic])
	require.Len(t, registry.command.AuditMoves, 1)
	assert.Equal(t, moderationrepo.AuditImageMove{OldObjectKey: oldPublic, NewObjectKey: auditKey}, registry.command.AuditMoves[0])
	assert.Contains(t, store.deletes, oldPublic)
}

func TestPublishCopyFailureSkipsDatabase(t *testing.T) {
	staging := "moderation/staging/moments/42/u1/sha.jpg"
	store := &publishStore{
		objects: map[string][]byte{staging: []byte("img")},
		copyErr: errors.New("garage unavailable"),
	}
	registry := &publishRegistry{}
	publisher := moderationmedia.NewPublisher(store, registry)

	_, err := publisher.Publish(context.Background(), moderationmedia.PublishCommand{
		ItemID: 10, RevisionID: 101, UserID: 42, MomentID: 8,
		Current: []moderationrepo.RevisionImageRecord{stagingImage(1, "sha", staging)},
	})

	require.Error(t, err)
	assert.False(t, registry.called)
}

func TestPublishDatabaseFailureDeletesNewFormalObjects(t *testing.T) {
	staging := "moderation/staging/moments/42/u1/sha.jpg"
	formal := "moments/42/8/sha.jpg"
	store := &publishStore{objects: map[string][]byte{staging: []byte("img")}}
	registry := &publishRegistry{resultErr: errors.New("db down")}
	publisher := moderationmedia.NewPublisher(store, registry)

	_, err := publisher.Publish(context.Background(), moderationmedia.PublishCommand{
		ItemID: 10, RevisionID: 101, UserID: 42, MomentID: 8,
		Current: []moderationrepo.RevisionImageRecord{stagingImage(1, "sha", staging)},
	})

	require.Error(t, err)
	require.True(t, registry.called)
	assert.Contains(t, store.deletes, formal)
	assert.NotContains(t, store.deletes, staging)
}

func TestPublishIdempotentSkipsExistingFormal(t *testing.T) {
	staging := "moderation/staging/moments/42/u1/sha.jpg"
	formal := "moments/42/8/sha.jpg"
	store := &publishStore{objects: map[string][]byte{
		staging: []byte("img"),
		formal:  []byte("img"),
	}}
	registry := &publishRegistry{}
	publisher := moderationmedia.NewPublisher(store, registry)

	_, err := publisher.Publish(context.Background(), moderationmedia.PublishCommand{
		ItemID: 10, RevisionID: 101, UserID: 42, MomentID: 8,
		Current: []moderationrepo.RevisionImageRecord{stagingImage(1, "sha", staging)},
	})

	require.NoError(t, err)
	require.True(t, registry.called)
	assert.Empty(t, store.copies)
	assert.Contains(t, store.deletes, staging)
}
