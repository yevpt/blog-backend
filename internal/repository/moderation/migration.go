package moderation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const LegacyUserPhase = "users"
const LegacyControlPhase = "control"

// MigrationRepository 是一次性历史审核迁移的数据边界。
type MigrationRepository interface {
	ListLegacyRecords(ctx context.Context, subjectType SubjectType, afterID uint64, limit int) ([]LegacyRecord, error)
	PersistLegacyRecords(ctx context.Context, records []LegacyRecord) error
	ListLegacyUsers(ctx context.Context, afterID uint64, limit int) ([]LegacyUser, error)
	PersistLegacyUsers(ctx context.Context, users []LegacyUser) error
	EnsureLegacyControl(ctx context.Context, registration RegistrationMode, publishing PublishingMode, now time.Time) error
	VerifyLegacy(ctx context.Context) (LegacyVerification, error)
}

// LegacyRecord 是迁移前读取的业务内容及其图片快照。
type LegacyRecord struct {
	Subject       SubjectRef
	AuthorID      uint64
	Content       string
	Visible       bool
	MomentOptions *MomentOptions
	ImageKeys     []string
	Images        []LegacyImage
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

// LegacyImage 是已读取原始字节并计算最终指纹的历史图片。
type LegacyImage struct {
	Seq       uint
	ObjectKey string
	SHA256    string
	MD5       string
	Size      uint64
	MediaType string
	IsGIF     bool
}

// LegacyUser 是需要回填 trusted + active 画像的历史用户。
type LegacyUser struct {
	UserID    uint64
	CreatedAt time.Time
}

// LegacyVerification 汇总迁移后仍缺失的事实数量。
type LegacyVerification struct {
	MissingItems    map[SubjectType]int64
	MissingProfiles int64
	MissingImages   int64
	MissingControl  int64
}

type legacySource struct {
	table         string
	rootColumn    string
	parentColumn  string
	authorColumn  string
	visibleColumn string
}

var legacySources = map[SubjectType]legacySource{
	SubjectMoment:              {table: "moment", authorColumn: "user_id", visibleColumn: "status"},
	SubjectArticleComment:      {table: "article_comment", rootColumn: "article_id", authorColumn: "user_id"},
	SubjectMomentComment:       {table: "moment_comment", rootColumn: "moment_id", authorColumn: "user_id"},
	SubjectGuestbook:           {table: "guestbook", rootColumn: "owner_user_id", authorColumn: "from_user_id"},
	SubjectArticleCommentReply: {table: "article_comment_reply", rootColumn: "comment_id", parentColumn: "parent_reply_id", authorColumn: "from_user_id"},
	SubjectMomentCommentReply:  {table: "moment_comment_reply", rootColumn: "comment_id", parentColumn: "parent_reply_id", authorColumn: "from_user_id"},
	SubjectGuestbookReply:      {table: "guestbook_reply", rootColumn: "comment_id", parentColumn: "parent_reply_id", authorColumn: "from_user_id"},
}

var legacySubjectTypes = []SubjectType{
	SubjectMoment, SubjectArticleComment, SubjectMomentComment, SubjectGuestbook,
	SubjectArticleCommentReply, SubjectMomentCommentReply, SubjectGuestbookReply,
}

// NewMigrationRepository 构造历史审核迁移仓储。
func NewMigrationRepository(db *gorm.DB) MigrationRepository { return &repository{db: db} }

// ListLegacyRecords 按业务类型和自增 ID 游标读取一批历史内容，包含软删除行。
func (r *repository) ListLegacyRecords(ctx context.Context, subjectType SubjectType, afterID uint64, limit int) ([]LegacyRecord, error) {
	source, ok := legacySources[subjectType]
	if !ok || limit <= 0 {
		return nil, ErrInvalidCommand
	}
	query := legacySelectQuery(subjectType, source)
	var rows []legacyRecordRow
	if err := r.db.WithContext(ctx).Raw(query, afterID, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	records := make([]LegacyRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, legacyRecord(subjectType, row))
	}
	if subjectType == SubjectMoment && len(records) > 0 {
		if err := r.loadLegacyMomentImages(ctx, records); err != nil {
			return nil, err
		}
	}
	return records, nil
}

type legacyRecordRow struct {
	ID                  uint64
	RootID              uint64
	ParentID            *uint64
	AuthorID            uint64
	Content             string
	Visible             bool
	MomentStatus        *uint8
	MomentCommentStatus *uint8
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}

func legacySelectQuery(subjectType SubjectType, source legacySource) string {
	root := "0"
	if source.rootColumn != "" {
		root = source.rootColumn
	}
	parent := "NULL"
	if source.parentColumn != "" {
		parent = source.parentColumn
	}
	visible, momentStatus, commentStatus := "1", "NULL", "NULL"
	if subjectType == SubjectMoment {
		visible, momentStatus, commentStatus = source.visibleColumn, "status", "comment_status"
	}
	return fmt.Sprintf(`SELECT id, %s AS root_id, %s AS parent_id, %s AS author_id,
content, %s AS visible, %s AS moment_status, %s AS moment_comment_status,
created_at, updated_at, deleted_at
FROM %s WHERE id > ? ORDER BY id ASC LIMIT ?`, root, parent, source.authorColumn, visible, momentStatus, commentStatus, source.table)
}

func legacyRecord(subjectType SubjectType, row legacyRecordRow) LegacyRecord {
	ref := SubjectRef{Type: subjectType, ID: row.ID, RootID: row.RootID, ParentID: row.ParentID}
	record := LegacyRecord{
		Subject: ref, AuthorID: row.AuthorID, Content: row.Content, Visible: row.Visible,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
	}
	if subjectType == SubjectMoment && row.MomentStatus != nil && row.MomentCommentStatus != nil {
		record.MomentOptions = &MomentOptions{Status: *row.MomentStatus, CommentStatus: *row.MomentCommentStatus}
	}
	return record
}

func (r *repository) loadLegacyMomentImages(ctx context.Context, records []LegacyRecord) error {
	ids := make([]uint64, 0, len(records))
	index := make(map[uint64]int, len(records))
	for i := range records {
		ids = append(ids, records[i].Subject.ID)
		index[records[i].Subject.ID] = i
	}
	var rows []struct {
		MomentID uint64
		URL      string
	}
	if err := r.db.WithContext(ctx).Table("moment_media").Select("moment_id", "url").
		Where("moment_id IN ? AND type = ? AND status = ? AND deleted_at IS NULL", ids, uint8(0), uint8(1)).
		Order("moment_id ASC, seq ASC, id ASC").Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		i, ok := index[row.MomentID]
		if ok && row.URL != "" {
			records[i].ImageKeys = append(records[i].ImageKeys, row.URL)
		}
	}
	return nil
}

// PersistLegacyRecords 在单事务内幂等创建审核项、正式版本、图片快照和全站图片通过记录。
func (r *repository) PersistLegacyRecords(ctx context.Context, records []LegacyRecord) error {
	if len(records) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, record := range records {
			if err := persistLegacyRecord(ctx, tx, record); err != nil {
				return err
			}
		}
		return nil
	})
}

func persistLegacyRecord(ctx context.Context, tx *gorm.DB, record LegacyRecord) error {
	if record.Subject.ID == 0 || record.AuthorID == 0 || !validSubjectType(record.Subject.Type) {
		return ErrInvalidCommand
	}
	item := legacyItem(record)
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Where("content_type = ? AND content_id = ?", record.Subject.Type, record.Subject.ID).
		Take(&item).Error; err != nil {
		return err
	}
	var existing model.ModerationRevision
	err := tx.WithContext(ctx).Where("item_id = ? AND version = ?", item.ID, uint64(1)).Take(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	revision := legacyRevision(item.ID, record)
	if err := tx.WithContext(ctx).Create(&revision).Error; err != nil {
		return err
	}
	if err := persistLegacyImages(ctx, tx, revision.ID, item.ID, record); err != nil {
		return err
	}
	updates := legacyItemPointers(record, revision.ID)
	return tx.WithContext(ctx).Model(&model.ModerationItem{}).Where("id = ?", item.ID).UpdateColumns(updates).Error
}

func legacyItem(record LegacyRecord) model.ModerationItem {
	lifecycle, publicState := string(LifecycleActive), string(PublicVisible)
	if !record.Visible {
		publicState = string(PublicHidden)
	}
	if record.DeletedAt != nil {
		lifecycle, publicState = string(LifecycleDeleted), string(PublicHidden)
	}
	return model.ModerationItem{
		ContentType: string(record.Subject.Type), ContentID: record.Subject.ID, AuthorID: record.AuthorID,
		LifecycleState: lifecycle, PublicState: publicState, LockVersion: 1,
		DeletedAt: record.DeletedAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func legacyRevision(itemID uint64, record LegacyRecord) model.ModerationRevision {
	decision := "legacy_migration"
	reviewedAt := record.CreatedAt
	revision := model.ModerationRevision{
		ItemID: itemID, Version: 1, SubmitterID: record.AuthorID,
		IdempotencyKey:   fmt.Sprintf("legacy_migration:%s:%d", record.Subject.Type, record.Subject.ID),
		SubmittedContent: record.Content, PublishedContent: record.Content,
		RiskLevel: string(RiskLow), PolicyAction: string(ActionAutoApprove), ReviewStatus: string(ReviewApproved),
		RulesetVersion: 0, RuleMatchIDs: "[]", DecisionType: &decision,
		ReviewedAt: &reviewedAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if record.MomentOptions != nil {
		status, commentStatus := record.MomentOptions.Status, record.MomentOptions.CommentStatus
		revision.MomentStatus, revision.MomentCommentStatus = &status, &commentStatus
	}
	return revision
}

func persistLegacyImages(ctx context.Context, tx *gorm.DB, revisionID, itemID uint64, record LegacyRecord) error {
	for _, image := range record.Images {
		approvedAt := record.CreatedAt
		global := model.ModerationImage{
			SHA256: image.SHA256, Size: image.Size, MD5: image.MD5, Status: ImageApproved,
			ApprovedAt: &approvedAt, LastUsedAt: record.UpdatedAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "sha256"}, {Name: "size"}},
			DoUpdates: clause.Assignments(map[string]any{
				"status": ImageApproved, "preview_object_key": nil,
				"approved_at":  gorm.Expr("COALESCE(approved_at, VALUES(approved_at))"),
				"last_used_at": gorm.Expr("GREATEST(last_used_at, VALUES(last_used_at))"),
				"updated_at":   gorm.Expr("GREATEST(updated_at, VALUES(updated_at))"),
			}),
		}).Create(&global).Error; err != nil {
			return err
		}
		revisionImage := model.ModerationRevisionImage{
			RevisionID: revisionID, Seq: image.Seq, ObjectKey: image.ObjectKey,
			SHA256: image.SHA256, MD5: image.MD5, Size: image.Size,
			MediaType: image.MediaType, IsGIF: image.IsGIF,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		}
		if err := tx.WithContext(ctx).Create(&revisionImage).Error; err != nil {
			return err
		}
		if record.Subject.Type != SubjectMoment && record.DeletedAt == nil {
			visible := model.ModerationVisibleImage{
				ItemID: itemID, RevisionID: revisionID, Seq: image.Seq, ObjectKey: image.ObjectKey,
				CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
			}
			if err := tx.WithContext(ctx).Create(&visible).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func legacyItemPointers(record LegacyRecord, revisionID uint64) map[string]any {
	updates := map[string]any{"approved_revision_id": revisionID, "updated_at": record.UpdatedAt}
	if record.DeletedAt == nil {
		updates["materialized_revision_id"] = revisionID
	}
	return updates
}

// ListLegacyUsers 按 ID 游标读取所有历史用户，包括已软删除账号。
func (r *repository) ListLegacyUsers(ctx context.Context, afterID uint64, limit int) ([]LegacyUser, error) {
	if limit <= 0 {
		return nil, ErrInvalidCommand
	}
	var rows []LegacyUser
	err := r.db.WithContext(ctx).Table("user").Select("id AS user_id", "created_at").
		Where("id > ?", afterID).Order("id ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// PersistLegacyUsers 幂等回填 trusted + active；已存在的新治理画像不被覆盖。
func (r *repository) PersistLegacyUsers(ctx context.Context, users []LegacyUser) error {
	if len(users) == 0 {
		return nil
	}
	rows := make([]model.UserModerationProfile, 0, len(users))
	for _, user := range users {
		rows = append(rows, model.UserModerationProfile{
			UserID: user.UserID, TrustLevel: model.ModerationTrustTrusted,
			TrustSource: model.ModerationTrustSourceAuto, SanctionState: model.ModerationSanctionActive,
			CreatedAt: user.CreatedAt, UpdatedAt: user.CreatedAt,
		})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// EnsureLegacyControl 确保全站控制单例存在，已有管理员设置不被覆盖。
func (r *repository) EnsureLegacyControl(ctx context.Context, registration RegistrationMode, publishing PublishingMode, now time.Time) error {
	if !validRegistrationMode(registration) || !validPublishingMode(publishing) || now.IsZero() {
		return ErrInvalidCommand
	}
	row := model.ModerationControl{
		ID: 1, RegistrationMode: string(registration),
		PublishingMode: string(publishing), ChangedAt: now,
		LockVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// VerifyLegacy 只读校验业务内容、用户、图片指纹和控制单例是否完整。
func (r *repository) VerifyLegacy(ctx context.Context) (LegacyVerification, error) {
	result := LegacyVerification{MissingItems: make(map[SubjectType]int64, len(legacySources))}
	for _, subjectType := range legacySubjectTypes {
		source := legacySources[subjectType]
		query := fmt.Sprintf(`SELECT COUNT(*) FROM %s AS source
LEFT JOIN moderation_item AS item ON item.content_type = ? AND item.content_id = source.id
		LEFT JOIN moderation_revision AS approved ON approved.id = item.approved_revision_id
WHERE item.id IS NULL
   OR approved.id IS NULL
   OR approved.review_status <> 'approved'
   OR approved.decision_type <> 'legacy_migration'
   OR item.pending_revision_id IS NOT NULL
   OR (item.lifecycle_state = 'active' AND (item.materialized_revision_id IS NULL OR item.materialized_revision_id <> item.approved_revision_id))
   OR (item.lifecycle_state = 'deleted' AND item.materialized_revision_id IS NOT NULL)`, source.table)
		var missing int64
		if err := r.db.WithContext(ctx).Raw(query, subjectType).Scan(&missing).Error; err != nil {
			return LegacyVerification{}, err
		}
		result.MissingItems[subjectType] = missing
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM user AS source
LEFT JOIN user_moderation_profile AS profile ON profile.user_id = source.id
WHERE profile.user_id IS NULL OR profile.trust_level <> 'trusted' OR profile.sanction_state <> 'active'`).Scan(&result.MissingProfiles).Error; err != nil {
		return LegacyVerification{}, err
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM moderation_revision_image AS revision_image
LEFT JOIN moderation_image AS image ON image.sha256 = revision_image.sha256 AND image.size = revision_image.size AND image.status = 'approved'
WHERE image.id IS NULL OR CHAR_LENGTH(revision_image.sha256) <> 64 OR CHAR_LENGTH(revision_image.md5) <> 32`).Scan(&result.MissingImages).Error; err != nil {
		return LegacyVerification{}, err
	}
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) = 0 FROM moderation_control WHERE id = 1").Scan(&result.MissingControl).Error; err != nil {
		return LegacyVerification{}, err
	}
	return result, nil
}
