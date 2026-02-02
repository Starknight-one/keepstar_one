package domain

import "time"

// TriggerType represents what initiated the delta
type TriggerType string

const (
	TriggerUserQuery    TriggerType = "USER_QUERY"
	TriggerWidgetAction TriggerType = "WIDGET_ACTION"
	TriggerSystem       TriggerType = "SYSTEM"
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
	Trigger   TriggerType            `json:"trigger"`
	Action    Action                 `json:"action"`
	Result    ResultMeta             `json:"result"`
	Template  map[string]interface{} `json:"template,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// StateMeta contains metadata for Agent 2
type StateMeta struct {
	Count   int               `json:"count"`
	Fields  []string          `json:"fields"`
	Aliases map[string]string `json:"aliases,omitempty"`
}

// StateData contains raw data (products, etc.)
type StateData struct {
	Products []Product `json:"products,omitempty"`
}

// StateCurrent represents the materialized current state
type StateCurrent struct {
	Data     StateData              `json:"data"`
	Meta     StateMeta              `json:"meta"`
	Template map[string]interface{} `json:"template,omitempty"`
}

// SessionState represents the full state for a chat session
type SessionState struct {
	ID        string       `json:"id"`
	SessionID string       `json:"session_id"`
	Current   StateCurrent `json:"current"`
	Step      int          `json:"step"` // Current step number
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}
