package moderation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/service/moderation"
)

var fixedTime = time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)

func TestTransitionVocabularyMatchesPersistence(t *testing.T) {
	assert.Equal(t, model.ModerationLifecycleActive, string(moderation.LifecycleActive))
	assert.Equal(t, model.ModerationLifecycleDeleted, string(moderation.LifecycleDeleted))
	assert.Equal(t, model.ModerationPublicVisible, string(moderation.PublicVisible))
	assert.Equal(t, model.ModerationPublicPlaceholder, string(moderation.PublicPlaceholder))
	assert.Equal(t, model.ModerationPublicHidden, string(moderation.PublicHidden))
	assert.Equal(t, model.ModerationPublicEmergencyHidden, string(moderation.PublicEmergencyHidden))
	assert.Equal(t, model.ModerationEventSubmit, string(moderation.EventSubmit))
	assert.Equal(t, model.ModerationEventResubmit, string(moderation.EventResubmit))
	assert.Equal(t, model.ModerationEventApprove, string(moderation.EventApprove))
	assert.Equal(t, model.ModerationEventCorrectAndApprove, string(moderation.EventCorrectAndApprove))
	assert.Equal(t, model.ModerationEventReject, string(moderation.EventReject))
	assert.Equal(t, model.ModerationEventDelete, string(moderation.EventDelete))
	assert.Equal(t, model.ModerationEventAdminDelete, string(moderation.EventAdminDelete))
	assert.Equal(t, model.ModerationEventEmergencyHide, string(moderation.EventEmergencyHide))
	assert.Equal(t, model.ModerationEventRestore, string(moderation.EventRestore))
}

func TestTransitionInitialSubmit(t *testing.T) {
	tests := []struct {
		name               string
		action             moderation.PolicyAction
		want               moderation.ItemSnapshot
		wantMaterializedID *uint64
	}{
		{name: "auto approve", action: moderation.ActionAutoApprove, want: activeSnapshot(moderation.PublicVisible, ptr(1), ptr(1), nil), wantMaterializedID: ptr(1)},
		{name: "post review", action: moderation.ActionPostReview, want: activeSnapshot(moderation.PublicVisible, ptr(1), nil, ptr(1)), wantMaterializedID: ptr(1)},
		{name: "pre review", action: moderation.ActionPreReview, want: activeSnapshot(moderation.PublicPlaceholder, nil, nil, ptr(1))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := moderation.Transition(moderation.TransitionInput{
				Previous: moderation.ItemSnapshot{}, Event: moderation.EventSubmit,
				Action: tt.action, NewRevisionID: 1, Now: fixedTime,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, plan.Item)
			assert.Equal(t, tt.wantMaterializedID, plan.MaterializeRevision)
			assert.Equal(t, moderation.EventSubmit, plan.AppendLog.Event)
			assert.Equal(t, ptr(1), plan.AppendLog.RevisionID)
			assert.False(t, plan.Idempotent)
		})
	}
}

func TestTransitionEditKeepsOrReplacesApprovedMaterialization(t *testing.T) {
	current := approvedSnapshot(10)

	postPlan, err := moderation.Transition(moderation.TransitionInput{
		Previous: current, Event: moderation.EventResubmit,
		Action: moderation.ActionPostReview, NewRevisionID: 11, Now: fixedTime,
	})
	require.NoError(t, err)
	assert.Equal(t, activeSnapshot(moderation.PublicVisible, ptr(11), ptr(10), ptr(11)), postPlan.Item)
	assert.Equal(t, ptr(11), postPlan.MaterializeRevision)

	prePlan, err := moderation.Transition(moderation.TransitionInput{
		Previous: current, Event: moderation.EventResubmit,
		Action: moderation.ActionPreReview, NewRevisionID: 12, Now: fixedTime,
	})
	require.NoError(t, err)
	assert.Equal(t, activeSnapshot(moderation.PublicVisible, ptr(10), ptr(10), ptr(12)), prePlan.Item)
	assert.Nil(t, prePlan.MaterializeRevision)

	autoPlan, err := moderation.Transition(moderation.TransitionInput{
		Previous: current, Event: moderation.EventResubmit,
		Action: moderation.ActionAutoApprove, NewRevisionID: 13, Now: fixedTime,
	})
	require.NoError(t, err)
	assert.Equal(t, activeSnapshot(moderation.PublicVisible, ptr(13), ptr(13), nil), autoPlan.Item)
	assert.Equal(t, ptr(13), autoPlan.MaterializeRevision)
}

func TestTransitionBlockDoesNotMutateState(t *testing.T) {
	current := approvedSnapshot(10)
	plan, err := moderation.Transition(moderation.TransitionInput{
		Previous: current, Event: moderation.EventResubmit,
		Action: moderation.ActionBlock, NewRevisionID: 11, Now: fixedTime,
	})

	assert.ErrorIs(t, err, moderation.ErrPolicyBlocked)
	assert.Equal(t, current, plan.Item)
	assert.Nil(t, plan.MaterializeRevision)
	assert.Nil(t, plan.SupersedeRevision)
	assert.Empty(t, plan.AppendLog.Event)
}

func TestTransitionResubmitSupersedesPendingRevision(t *testing.T) {
	current := activeSnapshot(moderation.PublicVisible, ptr(2), ptr(1), ptr(2))
	plan, err := moderation.Transition(moderation.TransitionInput{
		Previous: current, Event: moderation.EventResubmit,
		Action: moderation.ActionPreReview, NewRevisionID: 3, Now: fixedTime,
	})

	require.NoError(t, err)
	assert.Equal(t, ptr(2), plan.SupersedeRevision)
	assert.Equal(t, ptr(3), plan.Item.PendingRevisionID)
	assert.Equal(t, ptr(1), plan.Item.MaterializedRevisionID)
	assert.Equal(t, ptr(1), plan.Item.ApprovedRevisionID)
}

func TestTransitionRecognizesExactSubmissionReplay(t *testing.T) {
	tests := []struct {
		name    string
		current moderation.ItemSnapshot
		event   moderation.Event
		action  moderation.PolicyAction
		id      uint64
	}{
		{name: "initial post review", current: activeSnapshot(moderation.PublicVisible, ptr(1), nil, ptr(1)), event: moderation.EventSubmit, action: moderation.ActionPostReview, id: 1},
		{name: "edit post review", current: activeSnapshot(moderation.PublicVisible, ptr(2), ptr(1), ptr(2)), event: moderation.EventResubmit, action: moderation.ActionPostReview, id: 2},
		{name: "initial pre review", current: activeSnapshot(moderation.PublicPlaceholder, nil, nil, ptr(1)), event: moderation.EventSubmit, action: moderation.ActionPreReview, id: 1},
		{name: "edit pre review", current: activeSnapshot(moderation.PublicVisible, ptr(1), ptr(1), ptr(2)), event: moderation.EventResubmit, action: moderation.ActionPreReview, id: 2},
		{name: "auto approve", current: approvedSnapshot(3), event: moderation.EventSubmit, action: moderation.ActionAutoApprove, id: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := moderation.Transition(moderation.TransitionInput{
				Previous: tt.current, Event: tt.event,
				Action: tt.action, NewRevisionID: tt.id, Now: fixedTime,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.current, plan.Item)
			assert.True(t, plan.Idempotent)
			assert.Nil(t, plan.MaterializeRevision)
			assert.Nil(t, plan.SupersedeRevision)
			assert.Nil(t, plan.ReviewRevision)
			assert.Empty(t, plan.AppendLog.Event)
		})
	}
}

func TestTransitionRejectsRevisionIDCollision(t *testing.T) {
	tests := []struct {
		name    string
		current moderation.ItemSnapshot
		action  moderation.PolicyAction
		id      uint64
	}{
		{name: "pending auto approve", current: activeSnapshot(moderation.PublicVisible, ptr(2), ptr(1), ptr(2)), action: moderation.ActionAutoApprove, id: 2},
		{name: "pre snapshot with post action", current: activeSnapshot(moderation.PublicVisible, ptr(1), ptr(1), ptr(2)), action: moderation.ActionPostReview, id: 2},
		{name: "post snapshot with pre action", current: activeSnapshot(moderation.PublicVisible, ptr(2), ptr(1), ptr(2)), action: moderation.ActionPreReview, id: 2},
		{name: "approved id with auto action while pending", current: activeSnapshot(moderation.PublicVisible, ptr(2), ptr(1), ptr(2)), action: moderation.ActionAutoApprove, id: 1},
		{name: "approved post review", current: approvedSnapshot(3), action: moderation.ActionPostReview, id: 3},
		{name: "approved pre review", current: approvedSnapshot(3), action: moderation.ActionPreReview, id: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := moderation.Transition(moderation.TransitionInput{
				Previous: tt.current, Event: moderation.EventResubmit,
				Action: tt.action, NewRevisionID: tt.id, Now: fixedTime,
			})
			assert.ErrorIs(t, err, moderation.ErrRevisionCollision)
			assert.Equal(t, tt.current, plan.Item)
			assert.Nil(t, plan.MaterializeRevision)
			assert.Nil(t, plan.SupersedeRevision)
			assert.Empty(t, plan.AppendLog.Event)
		})
	}
}

func TestTransitionReviewEvents(t *testing.T) {
	current := activeSnapshot(moderation.PublicVisible, ptr(2), ptr(1), ptr(2))
	tests := []struct {
		event        moderation.Event
		decision     moderation.DecisionType
		wantItem     moderation.ItemSnapshot
		materialized *uint64
	}{
		{event: moderation.EventApprove, decision: moderation.DecisionApproved, wantItem: approvedSnapshot(2), materialized: ptr(2)},
		{event: moderation.EventCorrectAndApprove, decision: moderation.DecisionCorrected, wantItem: approvedSnapshot(2), materialized: ptr(2)},
		{event: moderation.EventReject, decision: moderation.DecisionRejected, wantItem: approvedSnapshot(1), materialized: ptr(1)},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			plan, err := moderation.Transition(moderation.TransitionInput{
				Previous: current, Event: tt.event, NewRevisionID: 2, Now: fixedTime,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantItem, plan.Item)
			assert.Equal(t, tt.materialized, plan.MaterializeRevision)
			require.NotNil(t, plan.ReviewRevision)
			assert.Equal(t, uint64(2), plan.ReviewRevision.RevisionID)
			assert.Equal(t, tt.decision, plan.ReviewRevision.Decision)
			assert.Equal(t, tt.event, plan.AppendLog.Event)
		})
	}
}

func TestTransitionRejectsFirstPendingSubmission(t *testing.T) {
	current := activeSnapshot(moderation.PublicVisible, ptr(1), nil, ptr(1))
	plan, err := moderation.Transition(moderation.TransitionInput{
		Previous: current, Event: moderation.EventReject, NewRevisionID: 1, Now: fixedTime,
	})

	require.NoError(t, err)
	assert.Equal(t, activeSnapshot(moderation.PublicHidden, nil, nil, nil), plan.Item)
	assert.Nil(t, plan.MaterializeRevision)
}

func TestTransitionDeleteFromEveryActiveState(t *testing.T) {
	visible := moderation.PublicVisible
	states := []struct {
		name string
		item moderation.ItemSnapshot
	}{
		{name: "initial post-review", item: activeSnapshot(moderation.PublicVisible, ptr(1), nil, ptr(1))},
		{name: "initial pre-review", item: activeSnapshot(moderation.PublicPlaceholder, nil, nil, ptr(1))},
		{name: "approved visible", item: approvedSnapshot(1)},
		{name: "low edit pending", item: activeSnapshot(moderation.PublicVisible, ptr(2), ptr(1), ptr(2))},
		{name: "medium edit pending", item: activeSnapshot(moderation.PublicVisible, ptr(1), ptr(1), ptr(2))},
		{name: "hidden", item: activeSnapshot(moderation.PublicHidden, nil, nil, nil)},
		{name: "emergency hidden", item: moderation.ItemSnapshot{
			LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicEmergencyHidden,
			MaterializedRevisionID: ptr(1), ApprovedRevisionID: ptr(1), StateBeforeEmergency: &visible,
			EmergencyHiddenReason: strPtr("incident"), EmergencyHiddenAt: timePtr(fixedTime.Add(-time.Hour)),
		}},
	}

	for _, event := range []moderation.Event{moderation.EventDelete, moderation.EventAdminDelete} {
		for _, state := range states {
			t.Run(string(event)+"/"+state.name, func(t *testing.T) {
				plan, err := moderation.Transition(moderation.TransitionInput{
					Previous: state.item, Event: event, Now: fixedTime,
				})
				require.NoError(t, err)
				assert.Equal(t, moderation.LifecycleDeleted, plan.Item.LifecycleState)
				assert.Equal(t, moderation.PublicHidden, plan.Item.PublicState)
				assert.Nil(t, plan.Item.MaterializedRevisionID)
				assert.Equal(t, state.item.ApprovedRevisionID, plan.Item.ApprovedRevisionID)
				assert.Nil(t, plan.Item.PendingRevisionID)
				assert.Nil(t, plan.Item.StateBeforeEmergency)
				assert.Nil(t, plan.Item.EmergencyHiddenReason)
				assert.Nil(t, plan.Item.EmergencyHiddenAt)
				assert.Equal(t, timePtr(fixedTime), plan.Item.DeletedAt)
				assert.Equal(t, state.item.PendingRevisionID, plan.SupersedeRevision)
				assert.Equal(t, event, plan.AppendLog.Event)
			})
		}
	}
}

func TestTransitionRepeatedDeleteIsIdempotent(t *testing.T) {
	current := deletedSnapshot()
	for _, event := range []moderation.Event{moderation.EventDelete, moderation.EventAdminDelete} {
		plan, err := moderation.Transition(moderation.TransitionInput{
			Previous: current, Event: event, Now: fixedTime.Add(time.Hour),
		})
		require.NoError(t, err)
		assert.Equal(t, current, plan.Item)
		assert.True(t, plan.Idempotent)
		assert.Empty(t, plan.AppendLog.Event)
	}
}

func TestDeletedIsTerminal(t *testing.T) {
	events := []moderation.Event{
		moderation.EventSubmit, moderation.EventResubmit,
		moderation.EventApprove, moderation.EventReject,
		moderation.EventCorrectAndApprove,
		moderation.EventEmergencyHide, moderation.EventRestore,
	}
	for _, event := range events {
		_, err := moderation.Transition(moderation.TransitionInput{
			Previous: deletedSnapshot(), Event: event, Now: fixedTime,
		})
		assert.ErrorIs(t, err, moderation.ErrAlreadyDeleted)
	}
}

func TestTransitionEmergencyHideAndRestore(t *testing.T) {
	current := approvedSnapshot(1)
	hidden, err := moderation.Transition(moderation.TransitionInput{
		Previous: current, Event: moderation.EventEmergencyHide, Reason: "incident", Now: fixedTime,
	})
	require.NoError(t, err)
	assert.Equal(t, moderation.PublicEmergencyHidden, hidden.Item.PublicState)
	assert.Equal(t, publicStatePtr(moderation.PublicVisible), hidden.Item.StateBeforeEmergency)
	assert.Equal(t, strPtr("incident"), hidden.Item.EmergencyHiddenReason)
	assert.Equal(t, timePtr(fixedTime), hidden.Item.EmergencyHiddenAt)

	repeated, err := moderation.Transition(moderation.TransitionInput{
		Previous: hidden.Item, Event: moderation.EventEmergencyHide,
		Reason: "overwrite", Now: fixedTime.Add(time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, hidden.Item, repeated.Item)
	assert.True(t, repeated.Idempotent)

	restored, err := moderation.Transition(moderation.TransitionInput{
		Previous: hidden.Item, Event: moderation.EventRestore, Now: fixedTime.Add(time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, current, restored.Item)
	assert.Nil(t, restored.Item.StateBeforeEmergency)
	assert.Nil(t, restored.Item.EmergencyHiddenReason)
	assert.Nil(t, restored.Item.EmergencyHiddenAt)
}

func TestTransitionEmergencyHideRequiresVisibleApprovedWithoutPending(t *testing.T) {
	states := []moderation.ItemSnapshot{
		activeSnapshot(moderation.PublicVisible, ptr(1), nil, ptr(1)),
		activeSnapshot(moderation.PublicPlaceholder, nil, nil, ptr(1)),
		activeSnapshot(moderation.PublicHidden, nil, nil, nil),
		activeSnapshot(moderation.PublicVisible, ptr(2), ptr(1), ptr(2)),
	}

	for _, current := range states {
		_, err := moderation.Transition(moderation.TransitionInput{
			Previous: current, Event: moderation.EventEmergencyHide, Now: fixedTime,
		})
		assert.ErrorIs(t, err, moderation.ErrInvalidTransition)
	}
}

func TestTransitionRestoreRequiresEmergencySnapshot(t *testing.T) {
	_, err := moderation.Transition(moderation.TransitionInput{
		Previous: approvedSnapshot(1), Event: moderation.EventRestore, Now: fixedTime,
	})
	assert.ErrorIs(t, err, moderation.ErrInvalidTransition)
}

func TestTransitionEmergencyHiddenRejectsSubmitAndResubmit(t *testing.T) {
	for _, event := range []moderation.Event{moderation.EventSubmit, moderation.EventResubmit} {
		t.Run(string(event), func(t *testing.T) {
			current := emergencySnapshot(1)
			plan, err := moderation.Transition(moderation.TransitionInput{
				Previous: current, Event: event,
				Action: moderation.ActionPostReview, NewRevisionID: 2, Now: fixedTime,
			})
			assert.ErrorIs(t, err, moderation.ErrInvalidTransition)
			assert.Equal(t, current, plan.Item)
		})
	}
}

func TestTransitionRejectsInvalidSnapshotCombinations(t *testing.T) {
	visible := moderation.PublicVisible
	states := []moderation.ItemSnapshot{
		{LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible},
		activeSnapshot(moderation.PublicPlaceholder, ptr(1), nil, ptr(1)),
		activeSnapshot(moderation.PublicVisible, ptr(9), ptr(1), ptr(2)),
		activeSnapshot(moderation.PublicVisible, ptr(1), ptr(1), ptr(1)),
		{
			LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible,
			MaterializedRevisionID: ptr(1), ApprovedRevisionID: ptr(1), StateBeforeEmergency: &visible,
		},
	}

	for _, current := range states {
		_, err := moderation.Transition(moderation.TransitionInput{
			Previous: current, Event: moderation.EventResubmit,
			Action: moderation.ActionPreReview, NewRevisionID: 3, Now: fixedTime,
		})
		assert.ErrorIs(t, err, moderation.ErrInvalidTransition)
	}
}

func TestTransitionDeleteRejectsInvalidPreviousSnapshot(t *testing.T) {
	current := activeSnapshot(moderation.PublicVisible, ptr(1), ptr(1), ptr(1))

	for _, event := range []moderation.Event{moderation.EventDelete, moderation.EventAdminDelete} {
		t.Run(string(event), func(t *testing.T) {
			plan, err := moderation.Transition(moderation.TransitionInput{
				Previous: current, Event: event, Now: fixedTime,
			})
			assert.ErrorIs(t, err, moderation.ErrInvalidTransition)
			assert.Equal(t, current, plan.Item)
			assert.Empty(t, plan.AppendLog.Event)
		})
	}
}

func TestTransitionRepeatedEmergencyHidePreservesEmptyReasonPointer(t *testing.T) {
	current := emergencySnapshot(1)
	empty := ""
	current.EmergencyHiddenReason = &empty

	plan, err := moderation.Transition(moderation.TransitionInput{
		Previous: current, Event: moderation.EventEmergencyHide,
		Reason: "overwrite", Now: fixedTime.Add(time.Hour),
	})
	require.NoError(t, err)
	require.NotNil(t, plan.Item.EmergencyHiddenReason)
	assert.Empty(t, *plan.Item.EmergencyHiddenReason)
}

func TestItemSnapshotDerivedDisplayVersionAndInteraction(t *testing.T) {
	tests := []struct {
		name        string
		item        moderation.ItemSnapshot
		display     moderation.DisplayVersion
		canInteract bool
	}{
		{name: "pending post review", item: activeSnapshot(moderation.PublicVisible, ptr(2), ptr(1), ptr(2)), display: moderation.DisplayPending},
		{name: "pending pre review edit", item: activeSnapshot(moderation.PublicVisible, ptr(1), ptr(1), ptr(2)), display: moderation.DisplayLastApproved},
		{name: "pending first pre review", item: activeSnapshot(moderation.PublicPlaceholder, nil, nil, ptr(1)), display: moderation.DisplayNone},
		{name: "approved", item: approvedSnapshot(1), display: moderation.DisplayLastApproved, canInteract: true},
		{name: "emergency hidden", item: emergencySnapshot(1), display: moderation.DisplayLastApproved},
		{name: "deleted", item: deletedSnapshot(), display: moderation.DisplayNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.display, tt.item.DisplayVersion())
			assert.Equal(t, tt.canInteract, tt.item.CanInteract())
		})
	}
}

func activeSnapshot(public moderation.PublicState, materialized, approved, pending *uint64) moderation.ItemSnapshot {
	return moderation.ItemSnapshot{
		LifecycleState: moderation.LifecycleActive, PublicState: public,
		MaterializedRevisionID: materialized, ApprovedRevisionID: approved, PendingRevisionID: pending,
	}
}

func approvedSnapshot(revisionID uint64) moderation.ItemSnapshot {
	return activeSnapshot(moderation.PublicVisible, ptr(revisionID), ptr(revisionID), nil)
}

func emergencySnapshot(revisionID uint64) moderation.ItemSnapshot {
	visible := moderation.PublicVisible
	return moderation.ItemSnapshot{
		LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicEmergencyHidden,
		MaterializedRevisionID: ptr(revisionID), ApprovedRevisionID: ptr(revisionID),
		StateBeforeEmergency: &visible, EmergencyHiddenReason: strPtr("incident"), EmergencyHiddenAt: timePtr(fixedTime),
	}
}

func deletedSnapshot() moderation.ItemSnapshot {
	return moderation.ItemSnapshot{
		LifecycleState: moderation.LifecycleDeleted, PublicState: moderation.PublicHidden,
		ApprovedRevisionID: ptr(1), DeletedAt: timePtr(fixedTime),
	}
}

func ptr(value uint64) *uint64 { return &value }

func strPtr(value string) *string { return &value }

func timePtr(value time.Time) *time.Time { return &value }

func publicStatePtr(value moderation.PublicState) *moderation.PublicState { return &value }
