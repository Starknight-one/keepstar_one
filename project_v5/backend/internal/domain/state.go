package domain

import "time"

// TemplatePresetInUseKey is the synthetic top-level key visual_assembly
// stamps onto the marshaled Document map when the preset path runs.
// The pipeline orchestrator reads it back to look up the drill-target
// preset in SystemAdjacency for the prefetch payload. Empty / absent
// for freestyle / modify call shapes (no preset).
const TemplatePresetInUseKey = "__presetInUse"

// CartItem is one entry in the user's cart zone.
type CartItem struct {
	EntityType EntityType `json:"entityType"`
	EntityId   string     `json:"entityId"`
	Quantity   int        `json:"quantity"`
}

// StateActions holds user-action state (likes, cart). Append-only via
// UpdateActions zone-write.
type StateActions struct {
	LikedIds  []string   `json:"likedIds"`
	CartItems []CartItem `json:"cartItems"`
}

// StateMeta describes the shape of the data zone for the renderer.
type StateMeta struct {
	Count        int               `json:"count"`
	ProductCount int               `json:"productCount,omitempty"`
	ServiceCount int               `json:"serviceCount,omitempty"`
	Fields       []string          `json:"fields"`
	Aliases      map[string]string `json:"aliases,omitempty"`
}

// StateData is the materialized data zone — products and services
// loaded by Agent1.
type StateData struct {
	Products []Product `json:"products,omitempty"`
	Services []Service `json:"services,omitempty"`
}

// StateCurrent is the currently materialized state. Template stores a
// scene-graph engine.Document JSON in V5 (in V4 it was a Formation).
type StateCurrent struct {
	Data     StateData              `json:"data"`
	Meta     StateMeta              `json:"meta"`
	Template map[string]interface{} `json:"template,omitempty"`
}

type ViewMode string

const (
	ViewModeGrid     ViewMode = "grid"
	ViewModeDetail   ViewMode = "detail"
	ViewModeList     ViewMode = "list"
	ViewModeCarousel ViewMode = "carousel"
)

// EntityRef points at one product or service.
type EntityRef struct {
	Type EntityType `json:"type"`
	ID   string     `json:"id"`
}

// ViewSnapshot captures a view state for back-navigation replay.
//
// Template stores the rendered scene-graph Document at the moment the
// snapshot was taken so /navigation/back can restore it without a
// re-render through Agent2. Mirrors V4's ViewSnapshot.Template
// behaviour. PresetInUse mirrors the active StateMeta.PresetInUse so
// the prefetch lookup after restoration uses the prior preset.
type ViewSnapshot struct {
	Mode        ViewMode               `json:"mode"`
	Focused     *EntityRef             `json:"focused,omitempty"`
	Refs        []EntityRef            `json:"refs"`
	Step        int                    `json:"step"`
	Template    map[string]interface{} `json:"template,omitempty"`
	PresetInUse string                 `json:"preset_in_use,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// ViewState is the current view configuration.
type ViewState struct {
	Mode    ViewMode   `json:"mode"`
	Focused *EntityRef `json:"focused,omitempty"`
}

// SessionState is the full state for a chat session — sectional zones plus
// step counter, conversation/agent2 history, and timestamps.
type SessionState struct {
	ID                  string         `json:"id"`
	SessionID           string         `json:"session_id"`
	Current             StateCurrent   `json:"current"`
	View                ViewState      `json:"view"`
	ViewStack           []ViewSnapshot `json:"view_stack"`
	Actions             StateActions   `json:"actions"`
	ConversationHistory []LLMMessage   `json:"conversation_history,omitempty"`
	Agent2History       []LLMMessage   `json:"agent2_history,omitempty"`
	Step                int            `json:"step"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}
