package moderation

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type reviewRecordRow struct {
	ItemID                 uint64
	ContentType            string
	ContentID              uint64
	AuthorID               uint64
	LockVersion            uint64
	LifecycleState         string
	PublicState            string
	MaterializedRevisionID *uint64
	ApprovedRevisionID     *uint64
	PendingRevisionID      *uint64
	StateBeforeEmergency   *string
	EmergencyHiddenReason  *string
	EmergencyHiddenAt      *time.Time
	DeletedAt              *time.Time
	RevisionID             uint64
	RevisionVersion        uint64
	SubmittedContent       string
	PublishedContent       string
	RiskLevel              string
	PolicyAction           string
	ReviewStatus           string
	MomentStatus           *uint8
	MomentCommentStatus    *uint8
	DecisionType           *string
	DecisionReason         *string
	ReviewerID             *uint64
	ReviewedAt             *time.Time
	CreatedAt              time.Time
}

const reviewRecordSelect = "moderation_item.id AS item_id, moderation_item.content_type, moderation_item.content_id, moderation_item.author_id, moderation_item.lock_version, moderation_item.lifecycle_state, moderation_item.public_state, moderation_item.materialized_revision_id, moderation_item.approved_revision_id, moderation_item.pending_revision_id, moderation_item.state_before_emergency, moderation_item.emergency_hidden_reason, moderation_item.emergency_hidden_at, moderation_item.deleted_at, moderation_revision.id AS revision_id, moderation_revision.version AS revision_version, moderation_revision.submitted_content, moderation_revision.published_content, moderation_revision.risk_level, moderation_revision.policy_action, moderation_revision.review_status, moderation_revision.moment_status, moderation_revision.moment_comment_status, moderation_revision.decision_type, moderation_revision.decision_reason, moderation_revision.reviewer_id, moderation_revision.reviewed_at, moderation_revision.created_at"

// ListReviewRecords 分页返回符合条件的审核版本，不在列表阶段读取业务关系。
func (r *repository) ListReviewRecords(ctx context.Context, filter ReviewFilter) (ReviewPage, error) {
	if err := validateReviewFilter(filter); err != nil {
		return ReviewPage{}, err
	}
	query := applyReviewFilter(r.reviewQuery(ctx), filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return ReviewPage{}, err
	}
	var rows []reviewRecordRow
	offset := (filter.Page - 1) * filter.PageSize
	if err := applyReviewFilter(r.reviewQuery(ctx), filter).
		Select(reviewRecordSelect).
		Order("moderation_revision.created_at DESC,moderation_revision.id DESC").
		Offset(offset).Limit(filter.PageSize).Scan(&rows).Error; err != nil {
		return ReviewPage{}, err
	}
	items := make([]ReviewRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapReviewRecord(row))
	}
	return ReviewPage{Total: total, Items: items}, nil
}

// LoadReviewRecord 读取明确版本，并为活动内容补齐不可伪造的业务父关系。
func (r *repository) LoadReviewRecord(ctx context.Context, itemID, revisionID uint64) (ReviewRecord, error) {
	if itemID == 0 || revisionID == 0 {
		return ReviewRecord{}, ErrInvalidCommand
	}
	var row reviewRecordRow
	err := r.reviewQuery(ctx).Select(reviewRecordSelect).
		Where("moderation_item.id = ? AND moderation_revision.id = ?", itemID, revisionID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ReviewRecord{}, ErrItemNotFound
	}
	if err != nil {
		return ReviewRecord{}, err
	}
	record := mapReviewRecord(row)
	if record.State.LifecycleState == LifecycleDeleted {
		return record, nil
	}
	adapter, err := r.adapter(record.Subject.Type)
	if err != nil {
		return ReviewRecord{}, err
	}
	snapshot, err := adapter.Load(ctx, r.db, record.Subject)
	if err != nil {
		return ReviewRecord{}, err
	}
	if snapshot.AuthorID != record.AuthorID {
		return ReviewRecord{}, ErrSubjectNotFound
	}
	record.Subject = snapshot.Ref
	return record, nil
}

// LoadCurrentReviewRecord 优先返回当前待审版本；没有待审版本时返回最新历史版本。
func (r *repository) LoadCurrentReviewRecord(ctx context.Context, itemID uint64) (ReviewRecord, error) {
	if itemID == 0 {
		return ReviewRecord{}, ErrInvalidCommand
	}
	var item struct {
		PendingRevisionID *uint64
	}
	err := r.db.WithContext(ctx).Table("moderation_item").Select("pending_revision_id").Where("id = ?", itemID).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ReviewRecord{}, ErrItemNotFound
	}
	if err != nil {
		return ReviewRecord{}, err
	}
	revisionID := uint64(0)
	if item.PendingRevisionID != nil {
		revisionID = *item.PendingRevisionID
	} else {
		err = r.db.WithContext(ctx).Table("moderation_revision").Select("id").
			Where("item_id = ?", itemID).Order("version DESC").Limit(1).Scan(&revisionID).Error
		if err != nil {
			return ReviewRecord{}, err
		}
		if revisionID == 0 {
			return ReviewRecord{}, ErrItemNotFound
		}
	}
	return r.LoadReviewRecord(ctx, itemID, revisionID)
}

func (r *repository) reviewQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("moderation_revision").
		Joins("JOIN moderation_item ON moderation_item.id = moderation_revision.item_id")
}

func applyReviewFilter(query *gorm.DB, filter ReviewFilter) *gorm.DB {
	if filter.ReviewStatus != nil {
		query = query.Where("moderation_revision.review_status = ?", *filter.ReviewStatus)
	}
	if filter.ContentType != nil {
		query = query.Where("moderation_item.content_type = ?", *filter.ContentType)
	}
	if filter.RiskLevel != nil {
		query = query.Where("moderation_revision.risk_level = ?", *filter.RiskLevel)
	}
	if filter.PublicState != nil {
		query = query.Where("moderation_item.public_state = ?", string(*filter.PublicState))
	}
	return query
}

func validPublicState(state PublicState) bool {
	return state == PublicVisible || state == PublicPlaceholder ||
		state == PublicHidden || state == PublicEmergencyHidden
}

func validateReviewFilter(filter ReviewFilter) error {
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 500 {
		return ErrInvalidCommand
	}
	if filter.ReviewStatus != nil {
		status := *filter.ReviewStatus
		if status != ReviewPending && status != ReviewApproved &&
			status != ReviewRejected && status != ReviewSuperseded {
			return ErrInvalidCommand
		}
	}
	if filter.ContentType != nil && !validSubjectType(*filter.ContentType) {
		return ErrInvalidCommand
	}
	if filter.RiskLevel != nil && *filter.RiskLevel != RiskLow && *filter.RiskLevel != RiskMedium && *filter.RiskLevel != RiskHigh {
		return ErrInvalidCommand
	}
	if filter.PublicState != nil && !validPublicState(*filter.PublicState) {
		return ErrInvalidCommand
	}
	return nil
}

func mapReviewRecord(row reviewRecordRow) ReviewRecord {
	record := ReviewRecord{
		ItemID: row.ItemID, Subject: SubjectRef{Type: SubjectType(row.ContentType), ID: row.ContentID},
		AuthorID: row.AuthorID, LockVersion: row.LockVersion,
		State: ItemState{
			LifecycleState: LifecycleState(row.LifecycleState), PublicState: PublicState(row.PublicState),
			Materialized: revisionRef(row.MaterializedRevisionID), Approved: revisionRef(row.ApprovedRevisionID),
			Pending: revisionRef(row.PendingRevisionID), EmergencyReason: row.EmergencyHiddenReason,
			EmergencyHiddenAt: row.EmergencyHiddenAt, DeletedAt: row.DeletedAt,
		},
		RevisionID: row.RevisionID, RevisionVersion: row.RevisionVersion,
		SubmittedContent: row.SubmittedContent, PublishedContent: row.PublishedContent,
		RiskLevel: RiskLevel(row.RiskLevel), PolicyAction: PolicyAction(row.PolicyAction),
		ReviewStatus: ReviewStatus(row.ReviewStatus), DecisionType: row.DecisionType,
		DecisionReason: row.DecisionReason, ReviewerID: row.ReviewerID, ReviewedAt: row.ReviewedAt,
		CreatedAt: row.CreatedAt,
	}
	if row.StateBeforeEmergency != nil {
		state := PublicState(*row.StateBeforeEmergency)
		record.State.StateBeforeEmergency = &state
	}
	if row.MomentStatus != nil && row.MomentCommentStatus != nil {
		record.MomentOptions = &MomentOptions{Status: *row.MomentStatus, CommentStatus: *row.MomentCommentStatus}
	}
	return record
}
