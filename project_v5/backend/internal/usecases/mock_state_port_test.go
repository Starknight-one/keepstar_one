package usecases

import (
	"context"
	"sort"

	"keepstar_v5/internal/domain"
)

// mockStatePort is an in-memory ports.StatePort for tests. Faithfully models
// the auto-step-assignment behaviour (MAX(step)+1 per session) and zone
// writes append a delta after the zone update — matching V4 semantics.
type mockStatePort struct {
	states map[string]*domain.SessionState
	deltas map[string][]domain.Delta
}

func newMockStatePort() *mockStatePort {
	return &mockStatePort{
		states: map[string]*domain.SessionState{},
		deltas: map[string][]domain.Delta{},
	}
}

func (m *mockStatePort) CreateState(_ context.Context, sessionID string) (*domain.SessionState, error) {
	s := &domain.SessionState{
		SessionID: sessionID,
		Current: domain.StateCurrent{
			Meta: domain.StateMeta{Aliases: map[string]string{}, Fields: []string{}},
		},
		View:      domain.ViewState{Mode: domain.ViewModeGrid},
		ViewStack: []domain.ViewSnapshot{},
		Actions:   domain.StateActions{LikedIds: []string{}, CartItems: []domain.CartItem{}},
	}
	m.states[sessionID] = s
	return s, nil
}

func (m *mockStatePort) GetState(_ context.Context, sessionID string) (*domain.SessionState, error) {
	s, ok := m.states[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	// Return a shallow copy so callers see a snapshot — matches Postgres
	// semantics where GetState rehydrates from JSONB and AddDelta touches
	// the DB row, not the caller's struct.
	cp := *s
	return &cp, nil
}

func (m *mockStatePort) UpdateState(_ context.Context, s *domain.SessionState) error {
	m.states[s.SessionID] = s
	return nil
}

func (m *mockStatePort) AddDelta(_ context.Context, sessionID string, delta *domain.Delta) (int, error) {
	step := 1
	for _, d := range m.deltas[sessionID] {
		if d.Step >= step {
			step = d.Step + 1
		}
	}
	delta.Step = step
	m.deltas[sessionID] = append(m.deltas[sessionID], *delta)
	if s, ok := m.states[sessionID]; ok {
		s.Step = step
	}
	return step, nil
}

func (m *mockStatePort) GetDeltas(_ context.Context, sessionID string) ([]domain.Delta, error) {
	return m.copyDeltas(sessionID, 0, 1<<31-1), nil
}
func (m *mockStatePort) GetDeltasSince(_ context.Context, sessionID string, fromStep int) ([]domain.Delta, error) {
	return m.copyDeltas(sessionID, fromStep, 1<<31-1), nil
}
func (m *mockStatePort) GetDeltasUntil(_ context.Context, sessionID string, toStep int) ([]domain.Delta, error) {
	return m.copyDeltas(sessionID, 0, toStep), nil
}

func (m *mockStatePort) copyDeltas(sessionID string, fromStep, toStep int) []domain.Delta {
	src := m.deltas[sessionID]
	out := make([]domain.Delta, 0, len(src))
	for _, d := range src {
		if d.Step >= fromStep && d.Step <= toStep {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Step < out[j].Step })
	return out
}

func (m *mockStatePort) UpdateData(ctx context.Context, sessionID string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error) {
	s := m.states[sessionID]
	s.Current.Data = data
	s.Current.Meta = meta
	return m.AddDelta(ctx, sessionID, info.ToDelta())
}
func (m *mockStatePort) UpdateTemplate(ctx context.Context, sessionID string, template map[string]interface{}, info domain.DeltaInfo) (int, error) {
	s := m.states[sessionID]
	s.Current.Template = template
	d := info.ToDelta()
	d.Template = template
	return m.AddDelta(ctx, sessionID, d)
}
func (m *mockStatePort) UpdateView(ctx context.Context, sessionID string, view domain.ViewState, stack []domain.ViewSnapshot, info domain.DeltaInfo) (int, error) {
	s := m.states[sessionID]
	s.View = view
	s.ViewStack = stack
	return m.AddDelta(ctx, sessionID, info.ToDelta())
}
func (m *mockStatePort) UpdateActions(ctx context.Context, sessionID string, actions domain.StateActions, info domain.DeltaInfo) (int, error) {
	s := m.states[sessionID]
	s.Actions = actions
	return m.AddDelta(ctx, sessionID, info.ToDelta())
}
func (m *mockStatePort) AppendConversation(_ context.Context, sessionID string, msgs []domain.LLMMessage) error {
	m.states[sessionID].ConversationHistory = msgs
	return nil
}
func (m *mockStatePort) AppendAgent2History(_ context.Context, sessionID string, msgs []domain.LLMMessage) error {
	m.states[sessionID].Agent2History = msgs
	return nil
}
func (m *mockStatePort) PushView(_ context.Context, sessionID string, snap *domain.ViewSnapshot) error {
	s := m.states[sessionID]
	s.ViewStack = append(s.ViewStack, *snap)
	return nil
}
func (m *mockStatePort) PopView(_ context.Context, sessionID string) (*domain.ViewSnapshot, error) {
	s := m.states[sessionID]
	if len(s.ViewStack) == 0 {
		return nil, nil
	}
	last := s.ViewStack[len(s.ViewStack)-1]
	s.ViewStack = s.ViewStack[:len(s.ViewStack)-1]
	return &last, nil
}
func (m *mockStatePort) GetViewStack(_ context.Context, sessionID string) ([]domain.ViewSnapshot, error) {
	return m.states[sessionID].ViewStack, nil
}
