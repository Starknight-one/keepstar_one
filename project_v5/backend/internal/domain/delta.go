package domain

import "time"

type TriggerType string

const (
	TriggerUserQuery    TriggerType = "USER_QUERY"
	TriggerWidgetAction TriggerType = "WIDGET_ACTION"
	TriggerSystem       TriggerType = "SYSTEM"
)

type DeltaSource string

const (
	SourceUser   DeltaSource = "user"
	SourceLLM    DeltaSource = "llm"
	SourceSystem DeltaSource = "system"
)

type DeltaType string

const (
	DeltaTypeAdd      DeltaType = "add"
	DeltaTypeRemove   DeltaType = "remove"
	DeltaTypeUpdate   DeltaType = "update"
	DeltaTypePush     DeltaType = "push"
	DeltaTypePop      DeltaType = "pop"
	DeltaTypeRollback DeltaType = "rollback"
)

type ActionType string

const (
	ActionSearch     ActionType = "SEARCH"
	ActionFilter     ActionType = "FILTER"
	ActionSort       ActionType = "SORT"
	ActionLayout     ActionType = "LAYOUT"
	ActionRollback   ActionType = "ROLLBACK"
	ActionClarify    ActionType = "CLARIFY"
	ActionLike       ActionType = "LIKE"
	ActionUnlike     ActionType = "UNLIKE"
	ActionCartAdd    ActionType = "CART_ADD"
	ActionCartRemove ActionType = "CART_REMOVE"
	ActionCartUpdate ActionType = "CART_UPDATE"

	// Entity-plane delta vocabulary (R28): entity mutations through the
	// operation plane use these — delta Path is "records" for mutations,
	// "data" for queries. There is NO generic OPERATION ActionType; notify
	// is audited via v5_operation_runs + v5_events only.
	ActionRecordCreate     ActionType = "RECORD_CREATE"
	ActionRecordUpdate     ActionType = "RECORD_UPDATE"
	ActionRecordTransition ActionType = "RECORD_TRANSITION"
	ActionEntityQuery      ActionType = "ENTITY_QUERY"
)

// RecordDeltaShape maps an entity-mutation operation kind to its R28 delta
// vocabulary (Path "records"): RECORD_CREATE for create_record +
// schedule_slot (R11: create semantics), RECORD_UPDATE, RECORD_TRANSITION.
// ok=false for every other kind — queries emit ENTITY_QUERY from the query
// executor's own zone write (Path "data"), notify has no delta. Shared by
// the invoke handler and the Agent1 chat path so both mutation doors stamp
// identical vocabulary.
func RecordDeltaShape(kind OperationKind) (ActionType, DeltaType, bool) {
	switch kind {
	case KindCreateRecord, KindScheduleSlot:
		return ActionRecordCreate, DeltaTypeAdd, true
	case KindUpdateRecord:
		return ActionRecordUpdate, DeltaTypeUpdate, true
	case KindTransitionStatus:
		return ActionRecordTransition, DeltaTypeUpdate, true
	}
	return "", "", false
}

type Action struct {
	Type   ActionType             `json:"type"`
	Tool   string                 `json:"tool,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
}

type ResultMeta struct {
	Count   int               `json:"count"`
	Fields  []string          `json:"fields"`
	Aliases map[string]string `json:"aliases,omitempty"`
}

type Delta struct {
	Step      int                    `json:"step"`
	TurnID    string                 `json:"turn_id,omitempty"`
	Trigger   TriggerType            `json:"trigger"`
	Source    DeltaSource            `json:"source"`
	ActorID   string                 `json:"actor_id"`
	DeltaType DeltaType              `json:"delta_type"`
	Path      string                 `json:"path"`
	Action    Action                 `json:"action"`
	Result    ResultMeta             `json:"result"`
	Template  map[string]interface{} `json:"template,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// DeltaInfo carries everything needed to materialize a Delta from a zone-write
// site. ToDelta() stamps CreatedAt and produces the Delta. Step is filled
// later by AddDelta (auto-assigned).
type DeltaInfo struct {
	TurnID    string      `json:"turn_id"`
	Trigger   TriggerType `json:"trigger"`
	Source    DeltaSource `json:"source"`
	ActorID   string      `json:"actor_id"`
	DeltaType DeltaType   `json:"delta_type"`
	Path      string      `json:"path"`
	Action    Action      `json:"action"`
	Result    ResultMeta  `json:"result"`
}

func (di DeltaInfo) ToDelta() *Delta {
	return &Delta{
		TurnID:    di.TurnID,
		Trigger:   di.Trigger,
		Source:    di.Source,
		ActorID:   di.ActorID,
		DeltaType: di.DeltaType,
		Path:      di.Path,
		Action:    di.Action,
		Result:    di.Result,
		CreatedAt: time.Now(),
	}
}
