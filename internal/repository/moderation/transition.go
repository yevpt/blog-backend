package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *repository) ApplyTransition(ctx context.Context, cmd ApplyTransitionCommand) (AppliedTransition, error) {
	if err := validateTransitionCommand(cmd); err != nil {
		return AppliedTransition{}, err
	}
	adapter, err := r.adapter(cmd.Subject.Type)
	if err != nil {
		return AppliedTransition{}, err
	}

	var applied AppliedTransition
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if cmd.Revision != nil {
			existing, err := lockIdempotencyScope(ctx, tx, cmd.Revision.SubmitterID, cmd.Revision.IdempotencyKey)
			if err != nil {
				return err
			}
			if existing != nil {
				if existing.Kind != ResultRevision {
					return ErrIdempotencyDomainConflict
				}
				applied = appliedTransitionFromStored(*existing)
				return nil
			}
		} else if cmd.ProfileChange != nil {
			if err := ensureAndLockProfile(ctx, tx, cmd.ProfileChange.UserID); err != nil {
				return err
			}
		}
		item, createdSubject, err := r.prepareItem(ctx, tx, adapter, &cmd)
		if err != nil {
			return err
		}
		applied.Subject = cmd.Subject
		applied.ItemID = item.ID
		if err := validateRevisionOwnership(ctx, tx, item, cmd); err != nil {
			return err
		}

		newRevision, err := createRevision(ctx, tx, item.ID, cmd.Revision)
		if err != nil {
			return err
		}
		if newRevision != nil {
			applied.RevisionID = newRevision.ID
			applied.RevisionVersion = newRevision.Version
		}
		if err := supersedeRevision(ctx, tx, item.ID, cmd.SupersedeRevisionID); err != nil {
			return err
		}
		if err := reviewRevision(ctx, tx, item.ID, cmd.Review); err != nil {
			return err
		}

		newRevisionID := applied.RevisionID
		if err := updateItem(ctx, tx, item, cmd.Next, newRevisionID); err != nil {
			return err
		}
		applied.LockVersion = item.LockVersion + 1

		if !createdSubject || !cmd.Materialize.IsNew {
			if err := r.materialize(ctx, tx, adapter, cmd, item.ID, newRevisionID); err != nil {
				return err
			}
		}
		if cmd.DeleteSubject {
			if err := r.deleteSubjectTree(ctx, tx, adapter, cmd.Subject, cmd.AuthorID); err != nil {
				return err
			}
		}
		if err := appendActionLog(ctx, tx, item.ID, newRevisionID, cmd.Log); err != nil {
			return err
		}
		return applyProfileChange(ctx, tx, cmd.ProfileChange)
	})
	return applied, err
}

func appliedTransitionFromStored(result StoredResult) AppliedTransition {
	replay := result
	return AppliedTransition{
		Subject: result.Subject, ItemID: result.ItemID, RevisionID: result.RevisionID,
		RevisionVersion: result.RevisionVersion, LockVersion: result.LockVersion,
		Replay: &replay,
	}
}

func validateTransitionCommand(cmd ApplyTransitionCommand) error {
	if !validSubjectType(cmd.Subject.Type) || cmd.AuthorID == 0 {
		return ErrInvalidCommand
	}
	if cmd.Subject.ID == 0 {
		if !cmd.CreateSubject || cmd.ExpectedLockVersion != 0 || cmd.ExpectedPendingID != nil || cmd.Revision == nil {
			return ErrInvalidCommand
		}
	} else if cmd.CreateSubject || cmd.ExpectedLockVersion == 0 {
		return ErrInvalidCommand
	}
	if err := validateTransitionSubjectRef(cmd.Subject); err != nil {
		return err
	}
	if cmd.Next.LifecycleState != LifecycleActive && cmd.Next.LifecycleState != LifecycleDeleted {
		return ErrInvalidCommand
	}
	if cmd.Next.PublicState != PublicVisible && cmd.Next.PublicState != PublicPlaceholder &&
		cmd.Next.PublicState != PublicHidden && cmd.Next.PublicState != PublicEmergencyHidden {
		return ErrInvalidCommand
	}
	if cmd.Revision != nil && (cmd.Revision.SubmitterID == 0 || cmd.Revision.IdempotencyKey == "" || cmd.Revision.ReviewStatus == "") {
		return ErrInvalidCommand
	}
	if cmd.ProfileChange != nil && cmd.ProfileChange.UserID == 0 {
		return ErrInvalidCommand
	}
	if cmd.Revision != nil && cmd.ProfileChange != nil && cmd.Revision.SubmitterID != cmd.ProfileChange.UserID {
		return ErrInvalidCommand
	}
	if referencesNewRevision(cmd) && cmd.Revision == nil {
		return ErrInvalidCommand
	}
	if err := validateRevisionRefs(cmd); err != nil {
		return err
	}
	return nil
}

func validateRevisionRefs(cmd ApplyTransitionCommand) error {
	refs := []RevisionRef{cmd.Next.Materialized, cmd.Next.Approved, cmd.Next.Pending, cmd.Materialize}
	if cmd.Log != nil {
		refs = append(refs, cmd.Log.Revision)
	}
	for _, ref := range refs {
		if ref.IsNew && ref.ID != 0 {
			return ErrInvalidCommand
		}
	}
	return nil
}

func validateTransitionSubjectRef(ref SubjectRef) error {
	switch ref.Type {
	case SubjectArticleCommentReply, SubjectMomentCommentReply, SubjectGuestbookReply:
		if ref.RootID == 0 || ref.ParentID == nil {
			return ErrInvalidCommand
		}
	case SubjectArticleComment, SubjectMomentComment, SubjectGuestbook:
		if ref.RootID == 0 || ref.ParentID != nil {
			return ErrInvalidCommand
		}
	case SubjectMoment:
		if ref.RootID != 0 || ref.ParentID != nil {
			return ErrInvalidCommand
		}
	default:
		return ErrInvalidCommand
	}
	return nil
}

func (r *repository) prepareItem(ctx context.Context, tx *gorm.DB, adapter subjectAdapter, cmd *ApplyTransitionCommand) (model.ModerationItem, bool, error) {
	if cmd.Subject.ID != 0 {
		item, err := lockItem(ctx, tx, cmd.Subject)
		if err != nil {
			return item, false, err
		}
		if err := validateLockedItem(item, *cmd); err != nil {
			return item, false, err
		}
		snapshot, err := adapter.Lock(ctx, tx, cmd.Subject, cmd.AuthorID)
		if err != nil {
			return item, false, err
		}
		if !sameSubjectRelation(snapshot.Ref, cmd.Subject) || snapshot.AuthorID != cmd.AuthorID {
			return item, false, ErrSubjectNotFound
		}
		return item, false, nil
	}
	if cmd.Materialize.ID != 0 && !cmd.Materialize.IsNew {
		return model.ModerationItem{}, false, ErrInvalidCommand
	}
	content := ""
	if cmd.Materialize.IsNew {
		content = cmd.Revision.PublishedContent
	}
	var assignedID uint64
	if err := adapter.Materialize(ctx, tx, MaterializeCommand{
		Ref: cmd.Subject, AuthorID: cmd.AuthorID, Content: content,
		Create: true, Visible: cmd.Materialize.IsNew, AssignedID: &assignedID,
	}); err != nil {
		return model.ModerationItem{}, false, err
	}
	cmd.Subject.ID = assignedID
	item := model.ModerationItem{
		ContentType: string(cmd.Subject.Type), ContentID: assignedID, AuthorID: cmd.AuthorID,
		LifecycleState: string(LifecycleActive), PublicState: string(PublicPlaceholder), LockVersion: 1,
	}
	if err := tx.WithContext(ctx).Create(&item).Error; err != nil {
		return model.ModerationItem{}, false, err
	}
	cmd.ExpectedLockVersion = item.LockVersion
	return item, true, nil
}

type revisionOwnerRow struct {
	ID     uint64
	ItemID uint64
}

func validateRevisionOwnership(ctx context.Context, tx *gorm.DB, item model.ModerationItem, cmd ApplyTransitionCommand) error {
	if cmd.SupersedeRevisionID != nil && !sameOptionalID(cmd.SupersedeRevisionID, item.PendingRevisionID) {
		return ErrPendingRevisionConflict
	}
	if cmd.Review != nil && (item.PendingRevisionID == nil || cmd.Review.RevisionID != *item.PendingRevisionID) {
		return ErrPendingRevisionConflict
	}
	ids := existingRevisionIDs(cmd)
	if len(ids) == 0 {
		return nil
	}
	var rows []revisionOwnerRow
	if err := tx.WithContext(ctx).Model(&model.ModerationRevision{}).
		Select("id", "item_id").Where("id IN ?", ids).Order("id ASC").
		Clauses(clause.Locking{Strength: "UPDATE"}).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(ids) {
		return ErrRevisionStateConflict
	}
	for _, row := range rows {
		if row.ItemID != item.ID {
			return ErrRevisionStateConflict
		}
	}
	return nil
}

func existingRevisionIDs(cmd ApplyTransitionCommand) []uint64 {
	unique := make(map[uint64]struct{})
	add := func(ref RevisionRef) {
		if !ref.IsNew && ref.ID != 0 {
			unique[ref.ID] = struct{}{}
		}
	}
	add(cmd.Next.Materialized)
	add(cmd.Next.Approved)
	add(cmd.Next.Pending)
	add(cmd.Materialize)
	if cmd.Log != nil {
		add(cmd.Log.Revision)
	}
	if cmd.SupersedeRevisionID != nil {
		unique[*cmd.SupersedeRevisionID] = struct{}{}
	}
	if cmd.Review != nil {
		unique[cmd.Review.RevisionID] = struct{}{}
	}
	ids := make([]uint64, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func referencesNewRevision(cmd ApplyTransitionCommand) bool {
	return cmd.Next.Materialized.IsNew || cmd.Next.Approved.IsNew || cmd.Next.Pending.IsNew ||
		cmd.Materialize.IsNew || (cmd.Log != nil && cmd.Log.Revision.IsNew)
}

func lockItem(ctx context.Context, tx *gorm.DB, ref SubjectRef) (model.ModerationItem, error) {
	var item model.ModerationItem
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("content_type = ? AND content_id = ?", ref.Type, ref.ID).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, ErrItemNotFound
	}
	return item, err
}

func validateLockedItem(item model.ModerationItem, cmd ApplyTransitionCommand) error {
	if item.AuthorID != cmd.AuthorID {
		return ErrSubjectNotFound
	}
	if item.LockVersion != cmd.ExpectedLockVersion {
		return ErrOptimisticLock
	}
	if !sameOptionalID(item.PendingRevisionID, cmd.ExpectedPendingID) {
		return ErrPendingRevisionConflict
	}
	return nil
}

func createRevision(ctx context.Context, tx *gorm.DB, itemID uint64, draft *RevisionDraft) (*model.ModerationRevision, error) {
	if draft == nil {
		return nil, nil
	}
	var maxVersion uint64
	if err := tx.WithContext(ctx).Raw("SELECT COALESCE(MAX(version), 0) FROM `moderation_revision` WHERE item_id = ?", itemID).Scan(&maxVersion).Error; err != nil {
		return nil, err
	}
	ruleIDs, err := json.Marshal(draft.RuleMatchIDs)
	if err != nil {
		return nil, err
	}
	row := model.ModerationRevision{
		ItemID: itemID, Version: maxVersion + 1, SubmitterID: draft.SubmitterID,
		IdempotencyKey: draft.IdempotencyKey, SubmittedContent: draft.SubmittedContent,
		PublishedContent: draft.PublishedContent, RiskLevel: string(draft.RiskLevel),
		PolicyAction: string(draft.PolicyAction), ReviewStatus: string(draft.ReviewStatus),
		RulesetVersion: draft.RulesetVersion, RuleMatchIDs: string(ruleIDs),
		DecisionType: draft.DecisionType, DecisionReason: draft.DecisionReason,
		ReviewerID: draft.ReviewerID, ReviewedAt: draft.ReviewedAt,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func supersedeRevision(ctx context.Context, tx *gorm.DB, itemID uint64, revisionID *uint64) error {
	if revisionID == nil {
		return nil
	}
	result := tx.WithContext(ctx).Model(&model.ModerationRevision{}).
		Where("id = ? AND item_id = ? AND review_status = ?", *revisionID, itemID, ReviewPending).
		Updates(map[string]any{"review_status": ReviewSuperseded, "updated_at": tx.NowFunc()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrRevisionStateConflict
	}
	return nil
}

func reviewRevision(ctx context.Context, tx *gorm.DB, itemID uint64, review *RevisionReview) error {
	if review == nil {
		return nil
	}
	updates := map[string]any{
		"review_status": review.Status, "decision_type": review.Decision,
		"decision_reason": review.Reason, "reviewer_id": review.ReviewerID,
		"reviewed_at": review.ReviewedAt, "updated_at": review.ReviewedAt,
	}
	result := tx.WithContext(ctx).Model(&model.ModerationRevision{}).
		Where("id = ? AND item_id = ? AND review_status = ?", review.RevisionID, itemID, ReviewPending).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrRevisionStateConflict
	}
	return nil
}

func updateItem(ctx context.Context, tx *gorm.DB, item model.ModerationItem, next ItemState, newRevisionID uint64) error {
	materialized, err := resolveRevisionRef(next.Materialized, newRevisionID)
	if err != nil {
		return err
	}
	approved, err := resolveRevisionRef(next.Approved, newRevisionID)
	if err != nil {
		return err
	}
	pending, err := resolveRevisionRef(next.Pending, newRevisionID)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"lifecycle_state": string(next.LifecycleState), "public_state": string(next.PublicState),
		"materialized_revision_id": materialized, "approved_revision_id": approved,
		"pending_revision_id": pending, "state_before_emergency": next.StateBeforeEmergency,
		"emergency_hidden_reason": next.EmergencyReason, "emergency_hidden_at": next.EmergencyHiddenAt,
		"deleted_at": next.DeletedAt, "lock_version": gorm.Expr("lock_version + 1"), "updated_at": tx.NowFunc(),
	}
	result := tx.WithContext(ctx).Model(&model.ModerationItem{}).
		Where("id = ? AND lock_version = ?", item.ID, item.LockVersion).UpdateColumns(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrOptimisticLock
	}
	return nil
}

func resolveRevisionRef(ref RevisionRef, newRevisionID uint64) (*uint64, error) {
	if ref.IsNew {
		if newRevisionID == 0 {
			return nil, ErrInvalidCommand
		}
		value := newRevisionID
		return &value, nil
	}
	if ref.ID == 0 {
		return nil, nil
	}
	value := ref.ID
	return &value, nil
}

func (r *repository) materialize(ctx context.Context, tx *gorm.DB, adapter subjectAdapter, cmd ApplyTransitionCommand, itemID, newRevisionID uint64) error {
	revisionID, err := resolveRevisionRef(cmd.Materialize, newRevisionID)
	if err != nil || revisionID == nil {
		return err
	}
	content := ""
	if cmd.Materialize.IsNew && cmd.Revision != nil {
		content = cmd.Revision.PublishedContent
	} else {
		var revision model.ModerationRevision
		if err := tx.WithContext(ctx).Where("id = ? AND item_id = ?", *revisionID, itemID).Take(&revision).Error; err != nil {
			return err
		}
		content = revision.PublishedContent
	}
	return adapter.Materialize(ctx, tx, MaterializeCommand{Ref: cmd.Subject, AuthorID: cmd.AuthorID, Content: content})
}

func (r *repository) deleteSubjectTree(ctx context.Context, tx *gorm.DB, adapter subjectAdapter, ref SubjectRef, authorID uint64) error {
	snapshot, err := adapter.Load(ctx, tx, ref)
	if err != nil {
		return err
	}
	if snapshot.AuthorID != authorID || !sameSubjectRelation(snapshot.Ref, ref) {
		return ErrSubjectNotFound
	}
	descendants, err := adapter.Descendants(ctx, tx, ref)
	if err != nil {
		return err
	}
	sortSubjectRefs(descendants)
	for _, descendant := range descendants {
		if err := tombstoneDescendant(ctx, tx, descendant); err != nil {
			return err
		}
		childAdapter, err := r.adapter(descendant.Type)
		if err != nil {
			return err
		}
		if err := childAdapter.Delete(ctx, tx, descendant); err != nil {
			return err
		}
	}
	return adapter.Delete(ctx, tx, ref)
}

func sameSubjectRelation(actual, expected SubjectRef) bool {
	if actual.Type != expected.Type || actual.ID != expected.ID {
		return false
	}
	if expected.Type == SubjectMoment {
		return true
	}
	return expected.RootID != 0 && actual.RootID == expected.RootID && sameOptionalID(actual.ParentID, expected.ParentID)
}

func tombstoneDescendant(ctx context.Context, tx *gorm.DB, ref SubjectRef) error {
	item, err := lockItem(ctx, tx, ref)
	if errors.Is(err, ErrItemNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if item.LifecycleState == string(LifecycleDeleted) {
		return nil
	}
	if err := supersedeRevision(ctx, tx, item.ID, item.PendingRevisionID); err != nil {
		return err
	}
	now := tx.NowFunc()
	result := tx.WithContext(ctx).Model(&model.ModerationItem{}).
		Where("id = ? AND lock_version = ?", item.ID, item.LockVersion).
		UpdateColumns(map[string]any{
			"lifecycle_state": LifecycleDeleted, "public_state": PublicHidden,
			"materialized_revision_id": nil, "pending_revision_id": nil,
			"state_before_emergency": nil, "emergency_hidden_reason": nil,
			"emergency_hidden_at": nil, "deleted_at": now,
			"lock_version": gorm.Expr("lock_version + 1"), "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrOptimisticLock
	}
	return nil
}

func appendActionLog(ctx context.Context, tx *gorm.DB, itemID, newRevisionID uint64, log *ActionLog) error {
	if log == nil {
		return nil
	}
	revisionID, err := resolveRevisionRef(log.Revision, newRevisionID)
	if err != nil {
		return err
	}
	row := model.ModerationActionLog{
		ItemID: &itemID, RevisionID: revisionID, ActorUserID: log.ActorUserID,
		SubjectUserID: log.SubjectUserID, Action: string(log.Action), Reason: log.Reason,
		MetadataJSON: log.MetadataJSON, CreatedAt: log.CreatedAt,
	}
	return tx.WithContext(ctx).Create(&row).Error
}

func applyProfileChange(ctx context.Context, tx *gorm.DB, change *ProfileChange) error {
	if change == nil {
		return nil
	}
	if change.UserID == 0 {
		return ErrInvalidCommand
	}
	updates := map[string]any{
		"clean_approval_streak": gorm.Expr("clean_approval_streak + ?", change.CleanApprovalDelta),
		"corrected_count":       gorm.Expr("corrected_count + ?", change.CorrectedDelta),
		"rejected_count":        gorm.Expr("rejected_count + ?", change.RejectedDelta),
		"high_risk_count":       gorm.Expr("high_risk_count + ?", change.HighRiskDelta),
		"violation_score":       gorm.Expr("violation_score + ?", change.ViolationScoreDelta),
		"updated_at":            change.UpdatedAt,
	}
	if change.TrustLevel != nil {
		updates["trust_level"] = *change.TrustLevel
	}
	if change.SanctionState != nil {
		updates["sanction_state"] = *change.SanctionState
	}
	result := tx.WithContext(ctx).Model(&model.UserModerationProfile{}).Where("user_id = ?", change.UserID).UpdateColumns(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%w: moderation profile missing", ErrInvalidCommand)
	}
	return nil
}

func sameOptionalID(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
