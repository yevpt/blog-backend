package moderation

import (
	"context"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
)

func (s *applicationService) AssertCanInteract(ctx context.Context, ref SubjectRef) error {
	item, err := s.repo.LoadItemState(ctx, ref)
	if err != nil {
		return err
	}
	if !itemSnapshot(item.State).CanInteract() {
		return ErrInteractionNotAllowed
	}
	_, err = s.repo.LoadSubject(ctx, ref)
	return err
}

func (s *applicationService) LoadViews(ctx context.Context, refs []SubjectRef, viewer Viewer) (map[SubjectKey]View, error) {
	return s.repo.LoadModerationView(ctx, refs, viewer)
}

func (s *applicationService) Delete(ctx context.Context, cmd DeleteCommand) error {
	if cmd.ActorID == 0 || cmd.Subject.ID == 0 {
		return ErrInvalidRequest
	}
	item, err := s.repo.LoadItemState(ctx, cmd.Subject)
	if err != nil {
		return err
	}
	if item.AuthorID != cmd.ActorID && !cmd.IsAdmin {
		return moderationrepo.ErrSubjectNotFound
	}
	event := EventDelete
	if cmd.IsAdmin {
		event = EventAdminDelete
	}
	plan, err := Transition(TransitionInput{
		Event: event, Previous: itemSnapshot(item.State), Reason: cmd.Reason, Now: s.now(),
	})
	if err != nil {
		return err
	}
	if plan.Idempotent {
		return nil
	}
	_, err = s.repo.ApplyTransition(ctx, moderationrepo.ApplyTransitionCommand{
		Subject: cmd.Subject, AuthorID: item.AuthorID,
		ExpectedLockVersion: item.LockVersion, ExpectedPendingID: existingID(item.State.Pending),
		Next: itemState(plan.Item), SupersedeRevisionID: plan.SupersedeRevision,
		DeleteSubject: true,
		Log: &moderationrepo.ActionLog{
			ActorUserID: &cmd.ActorID, SubjectUserID: &item.AuthorID,
			Action: moderationrepo.Event(event), Reason: plan.AppendLog.Reason,
			CreatedAt: plan.AppendLog.CreatedAt,
		},
	})
	return err
}
