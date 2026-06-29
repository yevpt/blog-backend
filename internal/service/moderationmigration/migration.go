// Package moderationmigration 提供可续跑的历史内容审核迁移和只读校验。
package moderationmigration

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/pkg/storage"
	"golang.org/x/net/html"
)

var (
	ErrInvalidCursor      = errors.New("invalid moderation migration cursor")
	ErrLegacyImageMissing = errors.New("legacy moderation image is missing")
	ErrVerificationFailed = errors.New("moderation migration verification failed")
)

const DonePhase = "done"

var contentPhases = []moderationrepo.SubjectType{
	moderationrepo.SubjectMoment,
	moderationrepo.SubjectArticleComment,
	moderationrepo.SubjectMomentComment,
	moderationrepo.SubjectGuestbook,
	moderationrepo.SubjectArticleCommentReply,
	moderationrepo.SubjectMomentCommentReply,
	moderationrepo.SubjectGuestbookReply,
}

// Cursor 唯一表示下一批迁移的类型和业务 ID 起点。
type Cursor struct {
	Type string `json:"type"`
	ID   uint64 `json:"id"`
}

// BatchResult 返回本批数量和可直接用于续跑的下一游标。
type BatchResult struct {
	Processed int    `json:"processed"`
	Next      Cursor `json:"next"`
	Done      bool   `json:"done"`
}

// Store 是迁移读取历史原图和解析对象 key 的最小边界。
type Store interface {
	storage.ImageObjectReader
	storage.ObjectKeyResolver
}

// Service 执行一批迁移或全局只读校验。
type Service struct {
	repo             moderationrepo.MigrationRepository
	store            Store
	now              func() time.Time
	registrationMode moderationrepo.RegistrationMode
	publishingMode   moderationrepo.PublishingMode
}

// Options 定义控制单例默认值和可测试时钟。
type Options struct {
	RegistrationMode moderationrepo.RegistrationMode
	PublishingMode   moderationrepo.PublishingMode
	Now              func() time.Time
}

// NewService 创建历史审核迁移服务。
func NewService(repo moderationrepo.MigrationRepository, store Store, options ...Options) *Service {
	now := time.Now
	registration, publishing := moderationrepo.RegistrationOpen, moderationrepo.PublishingOpen
	if len(options) > 0 {
		if options[0].Now != nil {
			now = options[0].Now
		}
		if options[0].RegistrationMode != "" {
			registration = options[0].RegistrationMode
		}
		if options[0].PublishingMode != "" {
			publishing = options[0].PublishingMode
		}
	}
	return &Service{
		repo: repo, store: store, now: now,
		registrationMode: registration, publishingMode: publishing,
	}
}

// RunBatch 只处理一个类型的一批记录；调用方保存 Next 即可断点续跑。
func (s *Service) RunBatch(ctx context.Context, cursor Cursor, limit int) (BatchResult, error) {
	if s == nil || s.repo == nil || limit <= 0 {
		return BatchResult{}, ErrInvalidCursor
	}
	phase := cursor.Type
	if phase == "" {
		phase = string(contentPhases[0])
	}
	if phase == DonePhase {
		return BatchResult{Next: Cursor{Type: DonePhase}, Done: true}, nil
	}
	if subjectType, ok := migrationSubjectType(phase); ok {
		return s.runContentBatch(ctx, cursor, subjectType, limit)
	}
	switch phase {
	case moderationrepo.LegacyUserPhase:
		return s.runUserBatch(ctx, cursor, limit)
	case moderationrepo.LegacyControlPhase:
		if err := s.repo.EnsureLegacyControl(ctx, s.registrationMode, s.publishingMode, s.now()); err != nil {
			return BatchResult{}, err
		}
		return BatchResult{Processed: 1, Next: Cursor{Type: DonePhase}, Done: true}, nil
	default:
		return BatchResult{}, ErrInvalidCursor
	}
}

func (s *Service) runContentBatch(ctx context.Context, cursor Cursor, subjectType moderationrepo.SubjectType, limit int) (BatchResult, error) {
	records, err := s.repo.ListLegacyRecords(ctx, subjectType, cursor.ID, limit)
	if err != nil {
		return BatchResult{}, err
	}
	if len(records) == 0 {
		return BatchResult{Next: Cursor{Type: nextPhase(string(subjectType))}}, nil
	}
	prepared := make([]moderationrepo.LegacyRecord, len(records))
	for i := range records {
		prepared[i], err = s.prepareRecord(ctx, records[i])
		if err != nil {
			return BatchResult{}, err
		}
	}
	if err := s.repo.PersistLegacyRecords(ctx, prepared); err != nil {
		return BatchResult{}, err
	}
	lastID := prepared[len(prepared)-1].Subject.ID
	return BatchResult{Processed: len(prepared), Next: Cursor{Type: string(subjectType), ID: lastID}}, nil
}

func (s *Service) runUserBatch(ctx context.Context, cursor Cursor, limit int) (BatchResult, error) {
	users, err := s.repo.ListLegacyUsers(ctx, cursor.ID, limit)
	if err != nil {
		return BatchResult{}, err
	}
	if len(users) == 0 {
		return BatchResult{Next: Cursor{Type: moderationrepo.LegacyControlPhase}}, nil
	}
	if err := s.repo.PersistLegacyUsers(ctx, users); err != nil {
		return BatchResult{}, err
	}
	return BatchResult{
		Processed: len(users),
		Next:      Cursor{Type: moderationrepo.LegacyUserPhase, ID: users[len(users)-1].UserID},
	}, nil
}

func (s *Service) prepareRecord(ctx context.Context, record moderationrepo.LegacyRecord) (moderationrepo.LegacyRecord, error) {
	keys := append([]string(nil), record.ImageKeys...)
	if record.Subject.Type != moderationrepo.SubjectMoment {
		keys = append(keys, extractImageSources(record.Content)...)
	}
	record.Images = make([]moderationrepo.LegacyImage, 0, len(keys))
	for index, value := range keys {
		required := record.Subject.Type == moderationrepo.SubjectMoment || !storage.IsAbsoluteURL(strings.TrimSpace(value))
		key, managed, err := s.managedObjectKey(value, required)
		if err != nil {
			return moderationrepo.LegacyRecord{}, err
		}
		if !managed {
			continue
		}
		image, err := s.fingerprintImage(ctx, key, uint(index+1))
		if err != nil {
			return moderationrepo.LegacyRecord{}, err
		}
		record.Images = append(record.Images, image)
	}
	return record, nil
}

func (s *Service) managedObjectKey(value string, required bool) (string, bool, error) {
	if s.store == nil || strings.TrimSpace(value) == "" {
		if required {
			return "", false, fmt.Errorf("%w: cannot resolve %q", ErrLegacyImageMissing, value)
		}
		return "", false, nil
	}
	key, err := s.store.ObjectKey(value)
	if err != nil || strings.TrimSpace(key) == "" {
		if required {
			return "", false, fmt.Errorf("%w: cannot resolve %q", ErrLegacyImageMissing, value)
		}
		return "", false, nil
	}
	return key, true, nil
}

func (s *Service) fingerprintImage(ctx context.Context, key string, seq uint) (moderationrepo.LegacyImage, error) {
	data, err := s.store.GetImageObject(ctx, key)
	if err != nil {
		return moderationrepo.LegacyImage{}, fmt.Errorf("%w: %s: %v", ErrLegacyImageMissing, key, err)
	}
	shaSum, md5Sum := sha256.Sum256(data), md5.Sum(data)
	mediaType := http.DetectContentType(data)
	return moderationrepo.LegacyImage{
		Seq: seq, ObjectKey: key, SHA256: hex.EncodeToString(shaSum[:]), MD5: hex.EncodeToString(md5Sum[:]),
		Size: uint64(len(data)), MediaType: mediaType, IsGIF: mediaType == "image/gif",
	}, nil
}

// Verify 执行只读完整性校验；任一缺失计数非零都返回稳定错误。
func (s *Service) Verify(ctx context.Context) (moderationrepo.LegacyVerification, error) {
	if s == nil || s.repo == nil {
		return moderationrepo.LegacyVerification{}, ErrVerificationFailed
	}
	result, err := s.repo.VerifyLegacy(ctx)
	if err != nil {
		return moderationrepo.LegacyVerification{}, err
	}
	if result.MissingProfiles > 0 || result.MissingImages > 0 || result.MissingControl > 0 {
		return result, ErrVerificationFailed
	}
	for _, count := range result.MissingItems {
		if count > 0 {
			return result, ErrVerificationFailed
		}
	}
	return result, nil
}

func migrationSubjectType(value string) (moderationrepo.SubjectType, bool) {
	for _, subjectType := range contentPhases {
		if value == string(subjectType) {
			return subjectType, true
		}
	}
	return "", false
}

func nextPhase(current string) string {
	for index, subjectType := range contentPhases {
		if current != string(subjectType) {
			continue
		}
		if index+1 < len(contentPhases) {
			return string(contentPhases[index+1])
		}
		return moderationrepo.LegacyUserPhase
	}
	return DonePhase
}

func extractImageSources(content string) []string {
	root, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil
	}
	result := make([]string, 0)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "img" {
			for _, attr := range node.Attr {
				if attr.Key == "src" && strings.TrimSpace(attr.Val) != "" {
					result = append(result, attr.Val)
					break
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return result
}
