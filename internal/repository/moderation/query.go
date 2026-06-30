package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *repository) LoadSubject(ctx context.Context, ref SubjectRef) (SubjectSnapshot, error) {
	adapter, err := r.adapter(ref.Type)
	if err != nil {
		return SubjectSnapshot{}, err
	}
	snapshot, err := adapter.Load(ctx, r.db, ref)
	if err != nil {
		return SubjectSnapshot{}, err
	}
	if (ref.RootID != 0 && ref.RootID != snapshot.Ref.RootID) ||
		(ref.ParentID != nil && !sameOptionalID(ref.ParentID, snapshot.Ref.ParentID)) {
		return SubjectSnapshot{}, ErrSubjectNotFound
	}
	return snapshot, nil
}

func (r *repository) LoadItemState(ctx context.Context, ref SubjectRef) (ItemStateRecord, error) {
	if !validSubjectType(ref.Type) || ref.ID == 0 {
		return ItemStateRecord{}, ErrInvalidCommand
	}
	var item model.ModerationItem
	err := r.db.WithContext(ctx).Where("content_type = ? AND content_id = ?", ref.Type, ref.ID).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ItemStateRecord{}, ErrItemNotFound
	}
	if err != nil {
		return ItemStateRecord{}, err
	}
	return itemStateRecord(item), nil
}

func itemStateRecord(item model.ModerationItem) ItemStateRecord {
	state := ItemState{
		LifecycleState: LifecycleState(item.LifecycleState), PublicState: PublicState(item.PublicState),
		Materialized: revisionRef(item.MaterializedRevisionID), Approved: revisionRef(item.ApprovedRevisionID),
		Pending: revisionRef(item.PendingRevisionID), EmergencyReason: cloneString(item.EmergencyHiddenReason),
		EmergencyHiddenAt: item.EmergencyHiddenAt, DeletedAt: item.DeletedAt,
	}
	if item.StateBeforeEmergency != nil {
		value := PublicState(*item.StateBeforeEmergency)
		state.StateBeforeEmergency = &value
	}
	return ItemStateRecord{ItemID: item.ID, AuthorID: item.AuthorID, State: state, LockVersion: item.LockVersion}
}

func revisionRef(id *uint64) RevisionRef {
	if id == nil {
		return RevisionRef{}
	}
	return ExistingRevision(*id)
}

func (r *repository) LoadPolicyContext(ctx context.Context, userID uint64) (PolicyContext, error) {
	if userID == 0 {
		return PolicyContext{}, ErrInvalidCommand
	}
	result := PolicyContext{TrustLevel: TrustNew, SanctionState: SanctionActive}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var control struct {
			PublishingMode string
			LockVersion    uint64
		}
		if err := tx.Table("moderation_control").Select("publishing_mode", "lock_version").
			Where("id = ?", uint64(1)).Take(&control).Error; err != nil {
			return err
		}
		result.PublishingMode = PublishingMode(control.PublishingMode)
		result.ControlVersion = control.LockVersion

		var row model.UserModerationProfile
		err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		profile := moderationProfileFromModel(row)
		updates := releaseExpiredProfile(&profile, tx.NowFunc())
		if len(updates) > 0 {
			updates["updated_at"] = tx.NowFunc()
			if err := tx.WithContext(ctx).Model(&model.UserModerationProfile{}).
				Where("user_id = ?", userID).UpdateColumns(updates).Error; err != nil {
				return err
			}
		}
		result.TrustLevel = profile.TrustLevel
		result.SanctionState = profile.SanctionState
		result.SanctionUntil = profile.SanctionUntil
		return nil
	})
	return result, err
}

type storedResultRow struct {
	Domain          string
	RecordID        uint64
	ItemID          *uint64
	AuthorID        uint64
	ContentType     string
	ContentID       *uint64
	ReviewStatus    string
	PublicState     string
	CreatedAt       time.Time
	RevisionVersion uint64
	LockVersion     uint64
	RiskLevel       string
	PolicyAction    string
	Content         string
	VisibleContent  string
}

const idempotencyResultQuery = "SELECT 'revision' AS domain, revision.id AS record_id, revision.item_id AS item_id, item.author_id, item.content_type, item.content_id, revision.review_status, item.public_state, revision.created_at, revision.version AS revision_version, item.lock_version AS lock_version, revision.risk_level, revision.policy_action, revision.published_content AS content, COALESCE(visible.published_content, '') AS visible_content FROM moderation_revision AS revision JOIN moderation_item AS item ON item.id = revision.item_id LEFT JOIN moderation_revision AS visible ON visible.id = item.materialized_revision_id AND visible.item_id = item.id WHERE revision.submitter_id = ? AND revision.idempotency_key = ? UNION ALL SELECT 'attempt' AS domain, attempt.id AS record_id, attempt.item_id AS item_id, attempt.user_id AS author_id, attempt.content_type, NULL AS content_id, 'blocked' AS review_status, '' AS public_state, attempt.created_at, 0 AS revision_version, 0 AS lock_version, 'high' AS risk_level, 'block' AS policy_action, '' AS content, '' AS visible_content FROM moderation_attempt AS attempt WHERE attempt.user_id = ? AND attempt.idempotency_key = ?"

func (r *repository) FindResultByIdempotencyKey(ctx context.Context, userID uint64, key string) (*StoredResult, error) {
	if userID == 0 || key == "" {
		return nil, ErrInvalidCommand
	}
	return findResultByIdempotencyKey(ctx, r.db, userID, key)
}

func findResultByIdempotencyKey(ctx context.Context, db *gorm.DB, userID uint64, key string) (*StoredResult, error) {
	var rows []storedResultRow
	if err := db.WithContext(ctx).Raw(idempotencyResultQuery, userID, key, userID, key).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 {
		return nil, ErrIdempotencyDomainConflict
	}
	return storedResult(rows[0]), nil
}

func storedResult(row storedResultRow) *StoredResult {
	result := &StoredResult{
		ItemID:          valueOrZero(row.ItemID),
		AuthorID:        row.AuthorID,
		Subject:         SubjectRef{Type: SubjectType(row.ContentType), ID: valueOrZero(row.ContentID)},
		ReviewStatus:    ReviewStatus(row.ReviewStatus),
		PublicState:     PublicState(row.PublicState),
		CreatedAt:       row.CreatedAt,
		RevisionVersion: row.RevisionVersion,
		LockVersion:     row.LockVersion,
		RiskLevel:       RiskLevel(row.RiskLevel),
		PolicyAction:    PolicyAction(row.PolicyAction),
		Content:         row.Content,
		VisibleContent:  row.VisibleContent,
	}
	if row.Domain == string(ResultBlocked) || row.Domain == "attempt" {
		result.Kind = ResultBlocked
		result.AttemptID = row.RecordID
		return result
	}
	result.Kind = ResultRevision
	result.RevisionID = row.RecordID
	return result
}

func lockIdempotencyScope(ctx context.Context, tx *gorm.DB, userID uint64, key string) (*StoredResult, error) {
	if err := ensureAndLockProfile(ctx, tx, userID); err != nil {
		return nil, err
	}
	return findResultByIdempotencyKey(ctx, tx, userID, key)
}

func ensureAndLockProfile(ctx context.Context, tx *gorm.DB, userID uint64) error {
	now := tx.NowFunc()
	if err := tx.WithContext(ctx).Exec(
		"INSERT INTO `user_moderation_profile` (`user_id`,`created_at`,`updated_at`) VALUES (?,?,?) ON DUPLICATE KEY UPDATE `user_id`=`user_id`",
		userID, now, now,
	).Error; err != nil {
		return err
	}
	var profile struct{ UserID uint64 }
	if err := tx.WithContext(ctx).Table("user_moderation_profile").Select("user_id").
		Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).Take(&profile).Error; err != nil {
		return err
	}
	return nil
}

func (r *repository) RecordBlockedAttempt(ctx context.Context, attempt BlockedAttempt) (StoredResult, error) {
	if attempt.UserID == 0 || attempt.IdempotencyKey == "" || !validSubjectType(attempt.SubjectType) {
		return StoredResult{}, ErrInvalidCommand
	}
	if attempt.ProfileChange != nil && attempt.ProfileChange.UserID != attempt.UserID {
		return StoredResult{}, ErrInvalidCommand
	}
	ruleIDs, err := json.Marshal(attempt.RuleMatchIDs)
	if err != nil {
		return StoredResult{}, err
	}
	row := model.ModerationAttempt{
		UserID: attempt.UserID, ContentType: string(attempt.SubjectType), ItemID: attempt.ItemID,
		IdempotencyKey: attempt.IdempotencyKey, RulesetVersion: attempt.RulesetVersion,
		RuleMatchIDs: string(ruleIDs), RuleMatchesTruncated: attempt.RuleMatchesTruncated, CreatedAt: attempt.CreatedAt,
	}
	var result StoredResult
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := lockIdempotencyScope(ctx, tx, attempt.UserID, attempt.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Kind != ResultBlocked {
				return ErrIdempotencyDomainConflict
			}
			result = *existing
			return nil
		}
		insert := tx.WithContext(ctx).Exec(
			"INSERT INTO `moderation_attempt` (`user_id`,`content_type`,`item_id`,`idempotency_key`,`ruleset_version`,`rule_match_ids`,`rule_matches_truncated`,`created_at`) VALUES (?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE `id`=`id`",
			row.UserID, SubjectType(row.ContentType), row.ItemID, row.IdempotencyKey, row.RulesetVersion, row.RuleMatchIDs, row.RuleMatchesTruncated, row.CreatedAt,
		)
		if insert.Error != nil {
			return insert.Error
		}
		if err := tx.WithContext(ctx).Where("user_id = ? AND idempotency_key = ?", attempt.UserID, attempt.IdempotencyKey).Take(&row).Error; err != nil {
			return err
		}
		if err := applyProfileChange(ctx, tx, attempt.ProfileChange); err != nil {
			return err
		}
		result = StoredResult{
			Kind: ResultBlocked, AttemptID: row.ID, ItemID: valueOrZero(row.ItemID),
			Subject: SubjectRef{Type: SubjectType(row.ContentType)}, CreatedAt: row.CreatedAt,
		}
		return nil
	})
	return result, err
}

type moderationViewRow struct {
	ContentType            string
	ContentID              uint64
	AuthorID               uint64
	LifecycleState         string
	PublicState            string
	MaterializedRevisionID *uint64
	ApprovedRevisionID     *uint64
	PendingRevisionID      *uint64
	MaterializedContent    *string
	PendingContent         *string
	PendingRiskLevel       *string
	PendingReviewStatus    *string
	PendingRuleMatchIDs    *string
	RejectedRevisionID     *uint64
	RejectedContent        *string
}

const moderationViewSelect = `moderation_item.content_type, moderation_item.content_id, moderation_item.author_id, moderation_item.lifecycle_state, moderation_item.public_state, moderation_item.materialized_revision_id, moderation_item.approved_revision_id, moderation_item.pending_revision_id, materialized.published_content AS materialized_content, pending.submitted_content AS pending_content, pending.risk_level AS pending_risk_level, pending.review_status AS pending_review_status, pending.rule_match_ids AS pending_rule_match_ids, (
SELECT rejected.id FROM moderation_revision AS rejected
WHERE rejected.item_id = moderation_item.id AND rejected.review_status = 'rejected'
ORDER BY rejected.reviewed_at DESC, rejected.id DESC LIMIT 1
) AS rejected_revision_id, (
SELECT rejected.published_content FROM moderation_revision AS rejected
WHERE rejected.item_id = moderation_item.id AND rejected.review_status = 'rejected'
ORDER BY rejected.reviewed_at DESC, rejected.id DESC LIMIT 1
) AS rejected_content`

func (r *repository) LoadModerationView(ctx context.Context, refs []SubjectRef, viewer Viewer) (map[SubjectKey]View, error) {
	result := make(map[SubjectKey]View, len(refs))
	if len(refs) == 0 {
		return result, nil
	}
	if viewer.Role != ViewerPublic && viewer.Role != ViewerAuthor && viewer.Role != ViewerAdmin {
		return nil, ErrInvalidCommand
	}
	types, ids, requested, err := viewKeys(refs)
	if err != nil {
		return nil, err
	}
	var rows []moderationViewRow
	err = r.db.WithContext(ctx).Table("moderation_item").
		Select(moderationViewSelect).
		Joins("LEFT JOIN moderation_revision AS materialized ON materialized.id = moderation_item.materialized_revision_id AND materialized.item_id = moderation_item.id").
		Joins("LEFT JOIN moderation_revision AS pending ON pending.id = moderation_item.pending_revision_id AND pending.item_id = moderation_item.id").
		Where("moderation_item.content_type IN ? AND moderation_item.content_id IN ?", types, ids).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	imagesByRevision, err := r.loadModerationViewImages(ctx, rows)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		key := SubjectKey{ContentType: SubjectType(row.ContentType), ContentID: row.ContentID}
		_, ok := requested[key]
		if !ok {
			continue
		}
		view, err := projectView(row, viewer)
		if err != nil {
			return nil, fmt.Errorf("project moderation view %s/%d: %w", row.ContentType, row.ContentID, err)
		}
		if row.MaterializedRevisionID != nil {
			view.VisibleImages = imagesByRevision[*row.MaterializedRevisionID]
		}
		// 已物化且与最后通过版本一致时，公开侧应展示原图而非待审预览（即使 moderation_image 状态滞后）。
		if row.MaterializedRevisionID != nil && row.ApprovedRevisionID != nil &&
			*row.MaterializedRevisionID == *row.ApprovedRevisionID &&
			row.PublicState == string(PublicVisible) {
			view.VisibleImages = AuthorOriginalImageViews(view.VisibleImages)
		}
		canReadPending := viewer.Role == ViewerAdmin || (viewer.Role == ViewerAuthor && viewer.UserID == row.AuthorID)
		if canReadPending && row.PendingRevisionID != nil {
			view.PendingImages = AuthorOriginalImageViews(imagesByRevision[*row.PendingRevisionID])
		}
		if canReadPending && row.RejectedRevisionID != nil && row.PublicState == string(PublicHidden) && row.MaterializedRevisionID == nil {
			if len(view.VisibleImages) == 0 {
				view.VisibleImages = AuthorOriginalImageViews(imagesByRevision[*row.RejectedRevisionID])
			}
		}
		if canReadPending && row.PublicState == string(PublicPlaceholder) && row.MaterializedRevisionID == nil && row.PendingRevisionID != nil {
			view.VisibleImages = view.PendingImages
		}
		// 访客在中风险首次发布占位态下展示模糊预览图（保留原图比例，不含原图 URL）。
		if !canReadPending && row.PublicState == string(PublicPlaceholder) && row.MaterializedRevisionID == nil && row.PendingRevisionID != nil {
			view.VisibleImages = imagesByRevision[*row.PendingRevisionID]
		}
		result[key] = view
	}
	return result, nil
}

type moderationViewImageRow struct {
	ID               uint64
	RevisionID       uint64
	Seq              uint
	ObjectKey        string
	PreviewObjectKey *string
	Status           *string
	IsGIF            bool
}

func (r *repository) loadModerationViewImages(ctx context.Context, rows []moderationViewRow) (map[uint64][]ImageView, error) {
	revisionSet := make(map[uint64]struct{})
	for _, row := range rows {
		if row.MaterializedRevisionID != nil {
			revisionSet[*row.MaterializedRevisionID] = struct{}{}
		}
		if row.PendingRevisionID != nil {
			revisionSet[*row.PendingRevisionID] = struct{}{}
		}
		if row.RejectedRevisionID != nil {
			revisionSet[*row.RejectedRevisionID] = struct{}{}
		}
	}
	result := make(map[uint64][]ImageView, len(revisionSet))
	if len(revisionSet) == 0 {
		return result, nil
	}
	revisionIDs := make([]uint64, 0, len(revisionSet))
	for id := range revisionSet {
		revisionIDs = append(revisionIDs, id)
	}
	var imageRows []moderationViewImageRow
	err := r.db.WithContext(ctx).Raw(`
SELECT revision_image.id, revision_image.revision_id, revision_image.seq, revision_image.object_key,
       image_record.preview_object_key, image_record.status, revision_image.is_gif
FROM moderation_revision_image AS revision_image
LEFT JOIN moderation_image AS image_record
  ON image_record.sha256 = revision_image.sha256 AND image_record.size = revision_image.size
WHERE revision_image.revision_id IN ?
ORDER BY revision_image.revision_id ASC, revision_image.seq ASC, revision_image.id ASC`, revisionIDs).Scan(&imageRows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range imageRows {
		approved := row.Status != nil && *row.Status == ImageApproved
		displayKey := ""
		if approved {
			displayKey = row.ObjectKey
		} else if row.PreviewObjectKey != nil {
			displayKey = *row.PreviewObjectKey
		}
		result[row.RevisionID] = append(result[row.RevisionID], ImageView{
			RevisionImageID: row.ID, Seq: row.Seq, SourceObjectKey: row.ObjectKey, DisplayObjectKey: displayKey,
			Approved: approved, IsGIF: row.IsGIF,
		})
	}
	return result, nil
}

func viewKeys(refs []SubjectRef) ([]SubjectType, []uint64, map[SubjectKey]struct{}, error) {
	types := make([]SubjectType, 0, len(refs))
	ids := make([]uint64, 0, len(refs))
	requested := make(map[SubjectKey]struct{}, len(refs))
	for _, ref := range refs {
		if ref.ID == 0 || !validSubjectType(ref.Type) {
			return nil, nil, nil, ErrInvalidCommand
		}
		requested[ref.Key()] = struct{}{}
		types = append(types, ref.Type)
		ids = append(ids, ref.ID)
	}
	return types, ids, requested, nil
}

func projectView(row moderationViewRow, viewer Viewer) (View, error) {
	view := View{
		PublicState:        PublicState(row.PublicState),
		DisplayVersion:     displayVersion(row),
		HasPendingRevision: row.PendingRevisionID != nil,
		CanInteract:        row.LifecycleState == string(LifecycleActive) && row.PublicState == string(PublicVisible) && row.PendingRevisionID == nil && row.ApprovedRevisionID != nil,
	}
	if row.LifecycleState == string(LifecycleActive) && row.PublicState == string(PublicVisible) && row.MaterializedContent != nil {
		view.VisibleContent = *row.MaterializedContent
	}
	canReadPending := viewer.Role == ViewerAdmin || (viewer.Role == ViewerAuthor && viewer.UserID == row.AuthorID)
	if !canReadPending || row.PendingRevisionID == nil {
		if canReadPending && row.PublicState == string(PublicHidden) && row.MaterializedContent == nil && row.RejectedContent != nil {
			view.VisibleContent = *row.RejectedContent
			rejected := ReviewRejected
			view.LastReviewStatus = &rejected
		}
		return view, nil
	}
	view.PendingContent = cloneString(row.PendingContent)
	if row.PendingRiskLevel != nil {
		risk := RiskLevel(*row.PendingRiskLevel)
		view.PendingRiskLevel = &risk
	}
	if row.PendingReviewStatus != nil {
		status := ReviewStatus(*row.PendingReviewStatus)
		view.PendingReviewStatus = &status
	}
	if row.PendingRuleMatchIDs != nil {
		if err := json.Unmarshal([]byte(*row.PendingRuleMatchIDs), &view.PendingRuleMatchIDs); err != nil {
			return View{}, err
		}
	}
	// 中风险首次发布尚无 materialized 版本：作者需看到自己提交的正文。
	if row.PublicState == string(PublicPlaceholder) && row.MaterializedRevisionID == nil && row.PendingContent != nil {
		view.VisibleContent = *row.PendingContent
	}
	return view, nil
}

func displayVersion(row moderationViewRow) DisplayVersion {
	if row.LifecycleState != string(LifecycleActive) || row.PublicState != string(PublicVisible) || row.MaterializedRevisionID == nil {
		return DisplayNone
	}
	if sameID(row.MaterializedRevisionID, row.PendingRevisionID) {
		return DisplayPending
	}
	if sameID(row.MaterializedRevisionID, row.ApprovedRevisionID) {
		return DisplayLastApproved
	}
	return DisplayNone
}

func sameID(left, right *uint64) bool {
	return left != nil && right != nil && *left == *right
}

func validSubjectType(subjectType SubjectType) bool {
	switch subjectType {
	case SubjectMoment, SubjectArticleComment, SubjectMomentComment, SubjectGuestbook,
		SubjectArticleCommentReply, SubjectMomentCommentReply, SubjectGuestbookReply:
		return true
	default:
		return false
	}
}

func valueOrZero(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// AuthorOriginalImageViews 让作者看到自己上传的原图，而非审核预览或 GIF 占位图。
func AuthorOriginalImageViews(images []ImageView) []ImageView {
	if len(images) == 0 {
		return nil
	}
	out := make([]ImageView, len(images))
	for i, image := range images {
		out[i] = image
		if image.SourceObjectKey == "" {
			continue
		}
		out[i].DisplayObjectKey = image.SourceObjectKey
		out[i].Approved = true
	}
	return out
}
