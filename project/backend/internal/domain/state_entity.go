package domain

import "time"

// TriggerType represents what initiated the delta
type TriggerType string

const (
	TriggerUserQuery    TriggerType = "USER_QUERY"
	TriggerWidgetAction TriggerType = "WIDGET_ACTION"
	TriggerSystem       TriggerType = "SYSTEM"
)

// DeltaSource identifies who initiated the change
type DeltaSource string

const (
	SourceUser   DeltaSource = "user"   // User actions (clicks, back, expand)
	SourceLLM    DeltaSource = "llm"    // LLM/Agent actions (search, render)
	SourceSystem DeltaSource = "system" // System actions (cleanup, TTL)
)

// DeltaType identifies the type of state change
type DeltaType string

const (
	DeltaTypeAdd      DeltaType = "add"      // Add data to state
	DeltaTypeRemove   DeltaType = "remove"   // Remove data from state
	DeltaTypeUpdate   DeltaType = "update"   // Update existing data
	DeltaTypePush     DeltaType = "push"     // Push to ViewStack
	DeltaTypePop      DeltaType = "pop"      // Pop from ViewStack
	DeltaTypeRollback DeltaType = "rollback" // Rollback to previous step
)

// ActionType represents the type of action performed
type ActionType string

const (
	ActionSearch   ActionType = "SEARCH"
	ActionFilter   ActionType = "FILTER"
	ActionSort     ActionType = "SORT"
	ActionLayout   ActionType = "LAYOUT"
	ActionRollback ActionType = "ROLLBACK"
)

// Action represents what happened in a delta
type Action struct {
	Type   ActionType             `json:"type"`
	Tool   string                 `json:"tool,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// ResultMeta contains metadata about the result (not raw data)
type ResultMeta struct {
	Count   int               `json:"count"`
	Fields  []string          `json:"fields"`
	Aliases map[string]string `json:"aliases,omitempty"`
}

// Delta represents a single change in state history
type Delta struct {
	Step      int                    `json:"step"`
	TurnID    string                 `json:"turn_id,omitempty"`
	Trigger   TriggerType            `json:"trigger"`
	Source    DeltaSource            `json:"source"`     // Who initiated: user/llm/system
	ActorID   string                 `json:"actor_id"`   // Which actor: "agent1", "agent2", "user_click", "user_back"
	DeltaType DeltaType              `json:"delta_type"` // Type of change: add/remove/update/push/pop
	Path      string                 `json:"path"`       // What changed: "data.products", "view.mode", "viewStack"
	Action    Action                 `json:"action"`
	Result    ResultMeta             `json:"result"`
	Template  map[string]interface{} `json:"template,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// DeltaInfo contains metadata for creating a delta via zone-write.
// Use ToDelta() to convert to a full Delta.
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

// ToDelta converts DeltaInfo to a Delta with CreatedAt set to now.
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

// StateMeta contains metadata for Agent 2
type StateMeta struct {
	Count        int               `json:"count"`
	ProductCount int               `json:"productCount,omitempty"`
	ServiceCount int               `json:"serviceCount,omitempty"`
	Fields       []string          `json:"fields"`
	Aliases      map[string]string `json:"aliases,omitempty"`
}

// StateData contains raw data (products, services, etc.)
type StateData struct {
	Products []Product `json:"products,omitempty"`
	Services []Service `json:"services,omitempty"`
}

// StateCurrent represents the materialized current state
type StateCurrent struct {
	Data     StateData              `json:"data"`
	Meta     StateMeta              `json:"meta"`
	Template map[string]interface{} `json:"template,omitempty"`
}

// ViewMode represents how items are displayed
type ViewMode string

const (
	ViewModeGrid     ViewMode = "grid"
	ViewModeDetail   ViewMode = "detail"
	ViewModeList     ViewMode = "list"
	ViewModeCarousel ViewMode = "carousel"
)

// EntityRef is a reference to a product or service
type EntityRef struct {
	Type EntityType `json:"type"` // product, service
	ID   string     `json:"id"`
}

// ViewSnapshot captures a view state for back navigation
type ViewSnapshot struct {
	Mode      ViewMode    `json:"mode"`
	Focused   *EntityRef  `json:"focused,omitempty"` // Expanded item (if detail mode)
	Refs      []EntityRef `json:"refs"`              // What was shown
	Step      int         `json:"step"`              // Delta step when captured
	CreatedAt time.Time   `json:"created_at"`
}

// ViewState represents current view configuration
type ViewState struct {
	Mode    ViewMode   `json:"mode"`
	Focused *EntityRef `json:"focused,omitempty"`
}

// SessionState represents the full state for a chat session
type SessionState struct {
	ID                  string         `json:"id"`
	SessionID           string         `json:"session_id"`
	Current             StateCurrent   `json:"current"`
	View                ViewState      `json:"view"`                          // Current view configuration
	ViewStack           []ViewSnapshot `json:"view_stack"`                    // Navigation history for back/forward
	ConversationHistory []LLMMessage   `json:"conversation_history,omitempty"` // LLM conversation history for caching
	Step                int            `json:"step"`                          // Current step number
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}
