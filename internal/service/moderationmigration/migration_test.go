package moderationmigration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/service/moderationmigration"
	"github.com/vpt/blog-backend/pkg/storage"
)

type migrationRepositoryStub struct {
	records   []moderationrepo.LegacyRecord
	persisted []moderationrepo.LegacyRecord
	verify    moderationrepo.LegacyVerification
}

func (s *migrationRepositoryStub) ListLegacyRecords(context.Context, moderationrepo.SubjectType, uint64, int) ([]moderationrepo.LegacyRecord, error) {
	return s.records, nil
}

func (s *migrationRepositoryStub) PersistLegacyRecords(_ context.Context, records []moderationrepo.LegacyRecord) error {
	s.persisted = append(s.persisted, records...)
	return nil
}

func (s *migrationRepositoryStub) ListLegacyUsers(context.Context, uint64, int) ([]moderationrepo.LegacyUser, error) {
	return nil, nil
}

func (s *migrationRepositoryStub) PersistLegacyUsers(context.Context, []moderationrepo.LegacyUser) error {
	return nil
}

func (s *migrationRepositoryStub) EnsureLegacyControl(context.Context, moderationrepo.RegistrationMode, moderationrepo.PublishingMode, time.Time) error {
	return nil
}

func (s *migrationRepositoryStub) VerifyLegacy(context.Context) (moderationrepo.LegacyVerification, error) {
	return s.verify, nil
}

type migrationStoreStub struct {
	objects map[string][]byte
}

func (s *migrationStoreStub) GetImageObject(_ context.Context, key string) ([]byte, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, errors.New("missing")
	}
	return data, nil
}

func (s *migrationStoreStub) ObjectKey(value string) (string, error) { return value, nil }

func TestRunBatchFingerprintsImagesAndReturnsResumeCursor(t *testing.T) {
	repo := &migrationRepositoryStub{records: []moderationrepo.LegacyRecord{{
		Subject:  moderationrepo.SubjectRef{Type: moderationrepo.SubjectMoment, ID: 7},
		AuthorID: 42, Content: "旧碎语", CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ImageKeys: []string{"moments/a.jpg"},
	}}}
	store := &migrationStoreStub{objects: map[string][]byte{"moments/a.jpg": []byte("legacy-image")}}
	service := moderationmigration.NewService(repo, store)

	result, err := service.RunBatch(context.Background(), moderationmigration.Cursor{
		Type: string(moderationrepo.SubjectMoment), ID: 0,
	}, 1)

	require.NoError(t, err)
	require.Len(t, repo.persisted, 1)
	require.Len(t, repo.persisted[0].Images, 1)
	assert.Len(t, repo.persisted[0].Images[0].SHA256, 64)
	assert.Equal(t, uint64(len("legacy-image")), repo.persisted[0].Images[0].Size)
	assert.Equal(t, string(moderationrepo.SubjectMoment), result.Next.Type)
	assert.Equal(t, uint64(7), result.Next.ID)
}

func TestRunBatchMissingImageStopsBeforePersist(t *testing.T) {
	repo := &migrationRepositoryStub{records: []moderationrepo.LegacyRecord{{
		Subject:  moderationrepo.SubjectRef{Type: moderationrepo.SubjectMoment, ID: 7},
		AuthorID: 42, Content: "旧碎语", ImageKeys: []string{"moments/missing.jpg"},
	}}}
	service := moderationmigration.NewService(repo, &migrationStoreStub{objects: map[string][]byte{}})

	_, err := service.RunBatch(context.Background(), moderationmigration.Cursor{Type: "moment"}, 10)

	require.ErrorIs(t, err, moderationmigration.ErrLegacyImageMissing)
	assert.Empty(t, repo.persisted)
}

func TestRunBatchSkipsUnresolvableEmbeddedImageForComments(t *testing.T) {
	repo := &migrationRepositoryStub{records: []moderationrepo.LegacyRecord{{
		Subject:  moderationrepo.SubjectRef{Type: moderationrepo.SubjectGuestbook, ID: 9, RootID: 3},
		AuthorID: 42, Content: `<img src="asdf" onerror="alert(1)"><img src="comments/guestbook/3/images/real.jpg">`,
	}}}
	store := &migrationStoreStub{objects: map[string][]byte{"comments/guestbook/3/images/real.jpg": []byte("real")}}
	service := moderationmigration.NewService(repo, store)

	_, err := service.RunBatch(context.Background(), moderationmigration.Cursor{Type: "guestbook"}, 10)

	require.NoError(t, err)
	require.Len(t, repo.persisted, 1)
	require.Len(t, repo.persisted[0].Images, 1)
	assert.Equal(t, "comments/guestbook/3/images/real.jpg", repo.persisted[0].Images[0].ObjectKey)
	assert.Contains(t, repo.persisted[0].Content, `src="asdf"`)
}

func TestRunBatchExtractsEmbeddedCommentImagesInDocumentOrder(t *testing.T) {
	repo := &migrationRepositoryStub{records: []moderationrepo.LegacyRecord{{
		Subject:  moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 8, RootID: 2},
		AuthorID: 42, Content: `<p>正文</p><img src="comments/a.jpg"><img src="comments/b.gif">`,
	}}}
	store := &migrationStoreStub{objects: map[string][]byte{
		"comments/a.jpg": []byte("a"), "comments/b.gif": []byte("b"),
	}}
	service := moderationmigration.NewService(repo, store)

	_, err := service.RunBatch(context.Background(), moderationmigration.Cursor{Type: "article_comment"}, 10)

	require.NoError(t, err)
	require.Len(t, repo.persisted, 1)
	require.Len(t, repo.persisted[0].Images, 2)
	assert.Equal(t, "comments/a.jpg", repo.persisted[0].Images[0].ObjectKey)
	assert.Equal(t, "comments/b.gif", repo.persisted[0].Images[1].ObjectKey)
}

func TestVerifyRejectsIncompleteMigration(t *testing.T) {
	repo := &migrationRepositoryStub{verify: moderationrepo.LegacyVerification{MissingProfiles: 2}}
	service := moderationmigration.NewService(repo, &migrationStoreStub{})

	result, err := service.Verify(context.Background())

	require.ErrorIs(t, err, moderationmigration.ErrVerificationFailed)
	assert.Equal(t, int64(2), result.MissingProfiles)
}

var _ storage.ObjectKeyResolver = (*migrationStoreStub)(nil)
