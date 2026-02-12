package usecases

import (
	"context"
	"time"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/ports"
)

// SendMessageUseCase handles simple chat message sending
type SendMessageUseCase struct {
	llm        ports.LLMPort
	cache      ports.CachePort
	events     ports.EventPort
	log        *logger.Logger
	sessionTTL time.Duration
}

// NewSendMessageUseCase creates a new use case
func NewSendMessageUseCase(llm ports.LLMPort, cache ports.CachePort, events ports.EventPort, log *logger.Logger) *SendMessageUseCase {
	return &SendMessageUseCase{
		llm:        llm,
		cache:      cache,
		events:     events,
		log:        log,
		sessionTTL: domain.SessionTTL,
	}
}

// WithSessionTTL sets custom session TTL
func (uc *SendMessageUseCase) WithSessionTTL(ttl time.Duration) *SendMessageUseCase {
	uc.sessionTTL = ttl
	return uc
}

// SendMessageRequest represents the input for sending a message
type SendMessageRequest struct {
	SessionID string
	TenantID  string
	Message   string
}

// SendMessageResponse represents the output of sending a message
type SendMessageResponse struct {
	SessionID string
	Response  string
	LatencyMs int
}

// Execute sends a message and returns the response, saving to database
func (uc *SendMessageUseCase) Execute(ctx context.Context, req SendMessageRequest) (*SendMessageResponse, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("usecase.send_message")
		defer endSpan()
	}

	now := time.Now()
	isNewSession := false

	// Get or create session
	var session *domain.Session
	var err error

	if req.SessionID != "" && uc.cache != nil {
		session, err = uc.cache.GetSession(ctx, req.SessionID)
		if err != nil && err != domain.ErrSessionNotFound {
			return nil, err
		}

		// Check sliding TTL - if session expired, mark it closed and create new
		if session != nil && now.Sub(session.LastActivityAt) > uc.sessionTTL {
			// Mark old session as closed
			session.Status = domain.SessionStatusClosed
			session.EndedAt = &now
			session.UpdatedAt = now
			if err := uc.cache.SaveSession(ctx, session); err != nil {
				uc.log.Warn("failed to close expired session", "error", err)
			}

			// Track timeout event
			if uc.events != nil {
				uc.events.TrackEvent(ctx, &domain.ChatEvent{
					SessionID: session.ID,
					EventType: domain.EventSessionTimeout,
					EventData: map[string]any{
						"last_activity": session.LastActivityAt,
						"expired_after": uc.sessionTTL.String(),
					},
					CreatedAt: now,
				})
			}

			session = nil // Force new session creation
		}
	}

	if session == nil {
		isNewSession = true
		session = &domain.Session{
			ID:             uuid.New().String(),
			TenantID:       req.TenantID,
			Status:         domain.SessionStatusActive,
			Messages:       []domain.Message{},
			StartedAt:      now,
			LastActivityAt: now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}

	// Create user message
	userMsg := domain.Message{
		ID:        uuid.New().String(),
		SessionID: session.ID,
		Role:      domain.MessageRoleUser,
		Content:   req.Message,
		SentAt:    now,
		Timestamp: now,
	}
	session.Messages = append(session.Messages, userMsg)

	// Call LLM
	startLLM := time.Now()
	response, err := uc.llm.Chat(ctx, req.Message)
	latencyMs := int(time.Since(startLLM).Milliseconds())

	if err != nil {
		return nil, err
	}

	receivedAt := time.Now()

	// Create assistant message
	assistantMsg := domain.Message{
		ID:         uuid.New().String(),
		SessionID:  session.ID,
		Role:       domain.MessageRoleAssistant,
		Content:    response,
		LatencyMs:  latencyMs,
		SentAt:     receivedAt,
		ReceivedAt: &receivedAt,
		Timestamp:  receivedAt,
	}
	session.Messages = append(session.Messages, assistantMsg)

	// Update session
	session.LastActivityAt = receivedAt
	session.UpdatedAt = receivedAt

	// Save session first (before tracking events due to foreign key)
	if uc.cache != nil {
		if err := uc.cache.SaveSession(ctx, session); err != nil {
			uc.log.Warn("failed to save session", "error", err)
		}
	}

	// Track events after session is saved
	if uc.events != nil {
		if isNewSession {
			if err := uc.events.TrackEvent(ctx, &domain.ChatEvent{
				SessionID: session.ID,
				EventType: domain.EventChatOpened,
				CreatedAt: now,
			}); err != nil {
				uc.log.Warn("failed to track chat_opened event", "error", err)
			}
		}

		if err := uc.events.TrackEvent(ctx, &domain.ChatEvent{
			SessionID: session.ID,
			EventType: domain.EventMessageSent,
			EventData: map[string]any{"message_id": userMsg.ID},
			CreatedAt: now,
		}); err != nil {
			uc.log.Warn("failed to track message_sent event", "error", err)
		}

		if err := uc.events.TrackEvent(ctx, &domain.ChatEvent{
			SessionID: session.ID,
			EventType: domain.EventMessageReceived,
			EventData: map[string]any{
				"message_id": assistantMsg.ID,
				"latency_ms": latencyMs,
			},
			CreatedAt: receivedAt,
		}); err != nil {
			uc.log.Warn("failed to track message_received event", "error", err)
		}
	}

	return &SendMessageResponse{
		SessionID: session.ID,
		Response:  response,
		LatencyMs: latencyMs,
	}, nil
}
