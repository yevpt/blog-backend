package moderation

import (
	"fmt"
	"time"
)

// LifecycleState 表示审核项是否仍可发生业务变化。
type LifecycleState string

const (
	LifecycleActive  LifecycleState = "active"
	LifecycleDeleted LifecycleState = "deleted"
)

// PublicState 表示审核项当前对公众的展示形态。
type PublicState string

const (
	PublicVisible         PublicState = "visible"
	PublicPlaceholder     PublicState = "placeholder"
	PublicHidden          PublicState = "hidden"
	PublicEmergencyHidden PublicState = "emergency_hidden"
)

// Event 表示状态机接受的封闭业务事件。
type Event string

const (
	EventSubmit            Event = "submit"
	EventResubmit          Event = "resubmit"
	EventApprove           Event = "approve"
	EventCorrectAndApprove Event = "correct_and_approve"
	EventReject            Event = "reject"
	EventDelete            Event = "delete"
	EventAdminDelete       Event = "admin_delete"
	EventEmergencyHide     Event = "emergency_hide"
	EventRestore           Event = "restore"
)

// ReviewStatus 表示审核版本转换后的持久化状态。
type ReviewStatus string

const (
	ReviewPending    ReviewStatus = "pending"
	ReviewApproved   ReviewStatus = "approved"
	ReviewRejected   ReviewStatus = "rejected"
	ReviewSuperseded ReviewStatus = "superseded"
)

// DecisionType 表示人工或自动通过版本的决策事实。
type DecisionType string

const (
	DecisionApproved  DecisionType = "approved"
	DecisionCorrected DecisionType = "corrected"
	DecisionRejected  DecisionType = "rejected"
)

// DisplayVersion 表示公开读取应选择的版本来源。
type DisplayVersion string

const (
	DisplayPending      DisplayVersion = "pending"
	DisplayLastApproved DisplayVersion = "last_approved"
	DisplayNone         DisplayVersion = "none"
)

// ItemSnapshot 是状态机使用的审核项状态，不包含任何 ORM 行为。
type ItemSnapshot struct {
	LifecycleState         LifecycleState
	PublicState            PublicState
	MaterializedRevisionID *uint64
	ApprovedRevisionID     *uint64
	PendingRevisionID      *uint64
	StateBeforeEmergency   *PublicState
	EmergencyHiddenReason  *string
	EmergencyHiddenAt      *time.Time
	DeletedAt              *time.Time
}

// DisplayVersion 从版本指针派生当前公开读取来源。
func (item ItemSnapshot) DisplayVersion() DisplayVersion {
	if item.LifecycleState == LifecycleDeleted || item.MaterializedRevisionID == nil {
		return DisplayNone
	}
	if sameRevision(item.MaterializedRevisionID, item.PendingRevisionID) {
		return DisplayPending
	}
	if sameRevision(item.MaterializedRevisionID, item.ApprovedRevisionID) {
		return DisplayLastApproved
	}
	return DisplayNone
}

// CanInteract 仅允许无待审版本的已通过公开内容参与互动。
func (item ItemSnapshot) CanInteract() bool {
	return item.LifecycleState == LifecycleActive &&
		item.PublicState == PublicVisible &&
		item.PendingRevisionID == nil &&
		item.ApprovedRevisionID != nil
}

// TransitionInput 是一次状态转换的事件数据；Now 必须由调用方提供。
type TransitionInput struct {
	Event         Event
	Action        PolicyAction
	NewRevisionID uint64
	Previous      ItemSnapshot
	Reason        string
	Now           time.Time
}

// ActionLogIntent 描述事务内应追加的审核操作日志。
type ActionLogIntent struct {
	Event      Event
	RevisionID *uint64
	Reason     *string
	CreatedAt  time.Time
}

// ReviewRevisionIntent 描述待审版本应落库的审核结果。
type ReviewRevisionIntent struct {
	RevisionID uint64
	Status     ReviewStatus
	Decision   DecisionType
	ReviewedAt time.Time
}

// TransitionPlan 是不含回调的持久化数据意图。
type TransitionPlan struct {
	Item                ItemSnapshot
	MaterializeRevision *uint64
	SupersedeRevision   *uint64
	ReviewRevision      *ReviewRevisionIntent
	AppendLog           ActionLogIntent
	Idempotent          bool
}

// Transition 根据输入中的前置快照和事件生成原子持久化计划，不执行外部副作用。
func Transition(input TransitionInput) (TransitionPlan, error) {
	current := cloneItem(input.Previous)
	if err := validateItemSnapshot(current); err != nil {
		return TransitionPlan{Item: current}, err
	}

	// 删除态只允许幂等重复删除。
	if current.LifecycleState == LifecycleDeleted {
		if input.Event == EventDelete || input.Event == EventAdminDelete {
			return idempotentPlan(current), nil
		}
		return TransitionPlan{Item: current}, ErrAlreadyDeleted
	}

	// 事件按职责分发到短小的状态转换函数。
	var plan TransitionPlan
	var err error
	switch input.Event {
	case EventSubmit, EventResubmit:
		plan, err = transitionSubmission(current, input)
	case EventApprove, EventCorrectAndApprove, EventReject:
		plan, err = transitionReview(current, input)
	case EventDelete, EventAdminDelete:
		plan, err = transitionDelete(current, input)
	case EventEmergencyHide:
		plan, err = transitionEmergencyHide(current, input)
	case EventRestore:
		plan, err = transitionRestore(current, input)
	default:
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "unsupported event")
	}
	if err != nil || plan.Idempotent {
		return plan, err
	}
	if err := validateItemSnapshot(plan.Item); err != nil {
		return TransitionPlan{Item: current}, fmt.Errorf("%w: output snapshot: %v", ErrInvalidTransition, err)
	}
	return plan, nil
}

func transitionSubmission(current ItemSnapshot, input TransitionInput) (TransitionPlan, error) {
	// 紧急隐藏内容只能先恢复，禁止提交事件间接公开。
	if current.PublicState == PublicEmergencyHidden {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "emergency hidden item must be restored first")
	}
	// 高风险不创建或替换任何审核状态。
	if input.Action == ActionBlock {
		return TransitionPlan{Item: current}, ErrPolicyBlocked
	}
	if input.NewRevisionID == 0 {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "revision id is required")
	}
	if isExactSubmissionReplay(current, input.Action, input.NewRevisionID) {
		return idempotentPlan(current), nil
	}
	if hasRevisionID(current, input.NewRevisionID) {
		return TransitionPlan{Item: current}, ErrRevisionCollision
	}
	if input.Now.IsZero() {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "time is required")
	}

	// submit 仅创建新审核项，resubmit 仅更新活动审核项。
	isNew := current.LifecycleState == ""
	if (input.Event == EventSubmit && !isNew) || (input.Event == EventResubmit && isNew) {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "event does not match item existence")
	}
	if !isNew && current.LifecycleState != LifecycleActive {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "item is not active")
	}

	// 新提交替换旧待审版本，但保留最后通过版本供回退。
	item := cloneItem(current)
	if isNew {
		item = ItemSnapshot{LifecycleState: LifecycleActive}
	}
	plan := TransitionPlan{
		Item:              item,
		SupersedeRevision: cloneUint64(current.PendingRevisionID),
		AppendLog:         logIntent(input.Event, input.NewRevisionID, input.Reason, input.Now),
	}

	switch input.Action {
	case ActionAutoApprove:
		applyApprovedSubmission(&plan, input)
	case ActionPostReview:
		applyPostReviewSubmission(&plan, input)
	case ActionPreReview:
		applyPreReviewSubmission(&plan, input)
	default:
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "unsupported policy action")
	}
	return plan, nil
}

func applyApprovedSubmission(plan *TransitionPlan, input TransitionInput) {
	plan.Item.PublicState = PublicVisible
	plan.Item.MaterializedRevisionID = uint64Pointer(input.NewRevisionID)
	plan.Item.ApprovedRevisionID = uint64Pointer(input.NewRevisionID)
	plan.Item.PendingRevisionID = nil
	plan.MaterializeRevision = uint64Pointer(input.NewRevisionID)
	plan.ReviewRevision = &ReviewRevisionIntent{
		RevisionID: input.NewRevisionID,
		Status:     ReviewApproved,
		Decision:   DecisionApproved,
		ReviewedAt: input.Now,
	}
}

func applyPostReviewSubmission(plan *TransitionPlan, input TransitionInput) {
	plan.Item.PublicState = PublicVisible
	plan.Item.MaterializedRevisionID = uint64Pointer(input.NewRevisionID)
	plan.Item.PendingRevisionID = uint64Pointer(input.NewRevisionID)
	plan.MaterializeRevision = uint64Pointer(input.NewRevisionID)
}

func applyPreReviewSubmission(plan *TransitionPlan, input TransitionInput) {
	plan.Item.PendingRevisionID = uint64Pointer(input.NewRevisionID)
	if plan.Item.ApprovedRevisionID == nil {
		plan.Item.PublicState = PublicPlaceholder
		plan.Item.MaterializedRevisionID = nil
		return
	}
	plan.Item.PublicState = PublicVisible
	plan.Item.MaterializedRevisionID = cloneUint64(plan.Item.ApprovedRevisionID)
}

func transitionReview(current ItemSnapshot, input TransitionInput) (TransitionPlan, error) {
	if current.LifecycleState != LifecycleActive || current.PendingRevisionID == nil {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "pending revision is required")
	}
	if input.NewRevisionID == 0 || *current.PendingRevisionID != input.NewRevisionID {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "pending revision does not match")
	}
	if input.Now.IsZero() {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "time is required")
	}

	// 审核事件始终清空当前待审指针并记录精确版本。
	item := cloneItem(current)
	item.PendingRevisionID = nil
	plan := TransitionPlan{
		Item:      item,
		AppendLog: logIntent(input.Event, input.NewRevisionID, input.Reason, input.Now),
	}

	switch input.Event {
	case EventApprove:
		applyReviewApproval(&plan, input, DecisionApproved)
	case EventCorrectAndApprove:
		applyReviewApproval(&plan, input, DecisionCorrected)
	case EventReject:
		applyReviewRejection(&plan, input)
	}
	return plan, nil
}

func applyReviewApproval(plan *TransitionPlan, input TransitionInput, decision DecisionType) {
	plan.Item.PublicState = PublicVisible
	plan.Item.MaterializedRevisionID = uint64Pointer(input.NewRevisionID)
	plan.Item.ApprovedRevisionID = uint64Pointer(input.NewRevisionID)
	plan.MaterializeRevision = uint64Pointer(input.NewRevisionID)
	plan.ReviewRevision = &ReviewRevisionIntent{
		RevisionID: input.NewRevisionID,
		Status:     ReviewApproved,
		Decision:   decision,
		ReviewedAt: input.Now,
	}
}

func applyReviewRejection(plan *TransitionPlan, input TransitionInput) {
	plan.ReviewRevision = &ReviewRevisionIntent{
		RevisionID: input.NewRevisionID,
		Status:     ReviewRejected,
		Decision:   DecisionRejected,
		ReviewedAt: input.Now,
	}
	if plan.Item.ApprovedRevisionID == nil {
		plan.Item.PublicState = PublicHidden
		plan.Item.MaterializedRevisionID = nil
		return
	}
	plan.Item.PublicState = PublicVisible
	plan.Item.MaterializedRevisionID = cloneUint64(plan.Item.ApprovedRevisionID)
	plan.MaterializeRevision = cloneUint64(plan.Item.ApprovedRevisionID)
}

func transitionDelete(current ItemSnapshot, input TransitionInput) (TransitionPlan, error) {
	if current.LifecycleState != LifecycleActive {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "item is not active")
	}
	if input.Now.IsZero() {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "time is required")
	}

	// 删除保留最后通过版本供审计，清空全部展示和紧急状态。
	item := cloneItem(current)
	item.LifecycleState = LifecycleDeleted
	item.PublicState = PublicHidden
	item.MaterializedRevisionID = nil
	item.PendingRevisionID = nil
	item.StateBeforeEmergency = nil
	item.EmergencyHiddenReason = nil
	item.EmergencyHiddenAt = nil
	item.DeletedAt = timePointer(input.Now)
	return TransitionPlan{
		Item:              item,
		SupersedeRevision: cloneUint64(current.PendingRevisionID),
		AppendLog:         logIntent(input.Event, 0, input.Reason, input.Now),
	}, nil
}

func transitionEmergencyHide(current ItemSnapshot, input TransitionInput) (TransitionPlan, error) {
	// 重复隐藏保持首次快照、原因和时间不变。
	if current.LifecycleState == LifecycleActive && current.PublicState == PublicEmergencyHidden {
		return idempotentPlan(current), nil
	}
	if current.LifecycleState != LifecycleActive ||
		current.PublicState != PublicVisible ||
		current.ApprovedRevisionID == nil ||
		current.PendingRevisionID != nil ||
		!sameRevision(current.MaterializedRevisionID, current.ApprovedRevisionID) {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "visible approved item without pending revision is required")
	}
	if input.Now.IsZero() {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "time is required")
	}

	// 首次隐藏只保存可恢复的 visible 快照。
	item := cloneItem(current)
	item.PublicState = PublicEmergencyHidden
	item.StateBeforeEmergency = publicStatePointer(PublicVisible)
	item.EmergencyHiddenReason = optionalStringPointer(input.Reason)
	item.EmergencyHiddenAt = timePointer(input.Now)
	return TransitionPlan{
		Item:      item,
		AppendLog: logIntent(input.Event, dereference(current.ApprovedRevisionID), input.Reason, input.Now),
	}, nil
}

func transitionRestore(current ItemSnapshot, input TransitionInput) (TransitionPlan, error) {
	if current.LifecycleState != LifecycleActive ||
		current.PublicState != PublicEmergencyHidden ||
		current.StateBeforeEmergency == nil ||
		*current.StateBeforeEmergency != PublicVisible {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "emergency snapshot is required")
	}
	if input.Now.IsZero() {
		return TransitionPlan{Item: current}, invalidTransition(input.Event, "time is required")
	}

	// 恢复只使用首次隐藏保存的快照并清空紧急字段。
	item := cloneItem(current)
	item.PublicState = *item.StateBeforeEmergency
	item.StateBeforeEmergency = nil
	item.EmergencyHiddenReason = nil
	item.EmergencyHiddenAt = nil
	return TransitionPlan{
		Item:      item,
		AppendLog: logIntent(input.Event, dereference(current.ApprovedRevisionID), input.Reason, input.Now),
	}, nil
}

func hasRevisionID(item ItemSnapshot, revisionID uint64) bool {
	return revisionMatches(item.MaterializedRevisionID, revisionID) ||
		revisionMatches(item.ApprovedRevisionID, revisionID) ||
		revisionMatches(item.PendingRevisionID, revisionID)
}

func isExactSubmissionReplay(item ItemSnapshot, action PolicyAction, revisionID uint64) bool {
	if item.LifecycleState != LifecycleActive {
		return false
	}

	switch action {
	case ActionPostReview:
		return item.PublicState == PublicVisible &&
			revisionMatches(item.PendingRevisionID, revisionID) &&
			revisionMatches(item.MaterializedRevisionID, revisionID)
	case ActionPreReview:
		if !revisionMatches(item.PendingRevisionID, revisionID) || revisionMatches(item.MaterializedRevisionID, revisionID) {
			return false
		}
		return (item.PublicState == PublicPlaceholder && item.MaterializedRevisionID == nil && item.ApprovedRevisionID == nil) ||
			(item.PublicState == PublicVisible && sameRevision(item.MaterializedRevisionID, item.ApprovedRevisionID))
	case ActionAutoApprove:
		return item.PublicState == PublicVisible && item.PendingRevisionID == nil &&
			revisionMatches(item.ApprovedRevisionID, revisionID) &&
			revisionMatches(item.MaterializedRevisionID, revisionID)
	default:
		return false
	}
}

func revisionMatches(pointer *uint64, revisionID uint64) bool {
	return pointer != nil && *pointer == revisionID
}

func idempotentPlan(item ItemSnapshot) TransitionPlan {
	return TransitionPlan{Item: cloneItem(item), Idempotent: true}
}

func logIntent(event Event, revisionID uint64, reason string, now time.Time) ActionLogIntent {
	intent := ActionLogIntent{Event: event, Reason: optionalStringPointer(reason), CreatedAt: now}
	if revisionID != 0 {
		intent.RevisionID = uint64Pointer(revisionID)
	}
	return intent
}

func invalidTransition(event Event, reason string) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalidTransition, event, reason)
}

func validateItemSnapshot(item ItemSnapshot) error {
	switch item.LifecycleState {
	case "":
		if item.PublicState != "" || hasAnyItemPointer(item) {
			return invalidTransition("", "new item snapshot contains persisted state")
		}
		return nil
	case LifecycleDeleted:
		if item.PublicState != PublicHidden || item.DeletedAt == nil ||
			item.MaterializedRevisionID != nil || item.PendingRevisionID != nil || hasEmergencyState(item) {
			return invalidTransition("", "deleted item violates terminal invariants")
		}
		return nil
	case LifecycleActive:
		if item.DeletedAt != nil {
			return invalidTransition("", "active item has deletion time")
		}
	default:
		return invalidTransition("", "unsupported lifecycle state")
	}
	if sameRevision(item.ApprovedRevisionID, item.PendingRevisionID) {
		return invalidTransition("", "approved and pending revisions must differ")
	}

	if item.PublicState == PublicEmergencyHidden {
		if item.PendingRevisionID != nil || item.StateBeforeEmergency == nil ||
			*item.StateBeforeEmergency != PublicVisible || item.EmergencyHiddenAt == nil ||
			!sameRevision(item.MaterializedRevisionID, item.ApprovedRevisionID) {
			return invalidTransition("", "emergency item violates snapshot invariants")
		}
		return nil
	}
	if hasEmergencyState(item) {
		return invalidTransition("", "non-emergency item retains emergency state")
	}

	switch item.PublicState {
	case PublicVisible:
		if !validVisiblePointers(item) {
			return invalidTransition("", "visible item has inconsistent revision pointers")
		}
	case PublicPlaceholder:
		if item.MaterializedRevisionID != nil || item.ApprovedRevisionID != nil || item.PendingRevisionID == nil {
			return invalidTransition("", "placeholder item has inconsistent revision pointers")
		}
	case PublicHidden:
		if item.MaterializedRevisionID != nil || item.ApprovedRevisionID != nil || item.PendingRevisionID != nil {
			return invalidTransition("", "hidden item has inconsistent revision pointers")
		}
	default:
		return invalidTransition("", "unsupported public state")
	}
	return nil
}

func validVisiblePointers(item ItemSnapshot) bool {
	if item.PendingRevisionID == nil {
		return sameRevision(item.MaterializedRevisionID, item.ApprovedRevisionID)
	}
	if item.ApprovedRevisionID == nil {
		return sameRevision(item.MaterializedRevisionID, item.PendingRevisionID)
	}
	return sameRevision(item.MaterializedRevisionID, item.ApprovedRevisionID) ||
		sameRevision(item.MaterializedRevisionID, item.PendingRevisionID)
}

func hasEmergencyState(item ItemSnapshot) bool {
	return item.StateBeforeEmergency != nil || item.EmergencyHiddenReason != nil || item.EmergencyHiddenAt != nil
}

func hasAnyItemPointer(item ItemSnapshot) bool {
	return item.MaterializedRevisionID != nil || item.ApprovedRevisionID != nil || item.PendingRevisionID != nil ||
		hasEmergencyState(item) || item.DeletedAt != nil
}

func cloneItem(item ItemSnapshot) ItemSnapshot {
	item.MaterializedRevisionID = cloneUint64(item.MaterializedRevisionID)
	item.ApprovedRevisionID = cloneUint64(item.ApprovedRevisionID)
	item.PendingRevisionID = cloneUint64(item.PendingRevisionID)
	item.StateBeforeEmergency = clonePublicState(item.StateBeforeEmergency)
	item.EmergencyHiddenReason = cloneString(item.EmergencyHiddenReason)
	item.EmergencyHiddenAt = cloneTime(item.EmergencyHiddenAt)
	item.DeletedAt = cloneTime(item.DeletedAt)
	return item
}

func sameRevision(left, right *uint64) bool {
	return left != nil && right != nil && *left == *right
}

func dereference(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

func uint64Pointer(value uint64) *uint64 { return &value }

func publicStatePointer(value PublicState) *PublicState { return &value }

func timePointer(value time.Time) *time.Time { return &value }

func optionalStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	return uint64Pointer(*value)
}

func clonePublicState(value *PublicState) *PublicState {
	if value == nil {
		return nil
	}
	return publicStatePointer(*value)
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return timePointer(*value)
}
