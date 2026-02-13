package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/ports"
)

// TestDB returns a connected postgres client with all migrations run.
// Skips the test if DATABASE_URL is not set.
func TestDB(t *testing.T) *postgres.Client {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("testutil.TestDB: connect failed: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("testutil.TestDB: chat migrations failed: %v", err)
	}
	if err := client.RunCatalogMigrations(ctx); err != nil {
		t.Fatalf("testutil.TestDB: catalog migrations failed: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("testutil.TestDB: state migrations failed: %v", err)
	}
	return client
}

// TestSession creates a chat_sessions row (FK parent for state) and registers cleanup.
func TestSession(t *testing.T, client *postgres.Client) string {
	t.Helper()
	ctx := context.Background()
	sessionID := uuid.New().String()
	tenantID := "test-tenant-" + sessionID[:8]

	_, err := client.Pool().Exec(ctx, `
		INSERT INTO chat_sessions (id, tenant_id, status)
		VALUES ($1, $2, 'active')
	`, sessionID, tenantID)
	if err != nil {
		t.Fatalf("testutil.TestSession: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.Pool().Exec(context.Background(),
			`DELETE FROM chat_session_state WHERE session_id = $1`, sessionID)
		_, _ = client.Pool().Exec(context.Background(),
			`DELETE FROM chat_deltas WHERE session_id = $1`, sessionID)
		_, _ = client.Pool().Exec(context.Background(),
			`DELETE FROM chat_sessions WHERE id = $1`, sessionID)
	})
	return sessionID
}

// TestStateWithProducts creates session + state + N seeded products, returns sessionID.
func TestStateWithProducts(t *testing.T, client *postgres.Client, n int) string {
	t.Helper()
	sessionID := TestSession(t, client)

	log := logger.New("error")
	adapter := postgres.NewStateAdapter(client, log)

	ctx := context.Background()
	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("testutil.TestStateWithProducts: create state: %v", err)
	}

	products := SeedProducts(n)
	data := domain.StateData{Products: products}
	meta := domain.StateMeta{
		Count:  n,
		Fields: []string{"id", "name", "price", "brand"},
	}
	info := domain.DeltaInfo{
		TurnID:    uuid.New().String(),
		Trigger:   domain.TriggerSystem,
		Source:    domain.SourceSystem,
		ActorID:   "test",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Result:    domain.ResultMeta{Count: n},
	}
	if _, err := adapter.UpdateData(ctx, sessionID, data, meta, info); err != nil {
		t.Fatalf("testutil.TestStateWithProducts: update data: %v", err)
	}
	return sessionID
}

// SeedProducts generates N test products.
func SeedProducts(n int) []domain.Product {
	products := make([]domain.Product, n)
	brands := []string{"Nike", "Adidas", "Puma", "Reebok"}
	categories := []string{"Sneakers", "Running", "Lifestyle", "Training"}

	for i := 0; i < n; i++ {
		products[i] = domain.Product{
			ID:             fmt.Sprintf("prod-%03d", i+1),
			TenantID:       "test-tenant",
			Name:           fmt.Sprintf("Test Product %d", i+1),
			Description:    fmt.Sprintf("Description for product %d", i+1),
			Price:          (i + 1) * 100_00, // in kopecks
			PriceFormatted: fmt.Sprintf("%d ₽", (i+1)*100),
			Currency:       "RUB",
			Brand:          brands[i%len(brands)],
			Category:       categories[i%len(categories)],
			Rating:         4.0 + float64(i%10)/10.0,
			StockQuantity:  10 + i,
			Images:         []string{fmt.Sprintf("https://img.test/%d.jpg", i+1)},
			Tags:           []string{"test", fmt.Sprintf("tag-%d", i%3)},
		}
	}
	return products
}

// SeedServices generates N test services.
func SeedServices(n int) []domain.Service {
	services := make([]domain.Service, n)
	providers := []string{"FitPro", "YogaSpace", "GymPlus"}

	for i := 0; i < n; i++ {
		services[i] = domain.Service{
			ID:           fmt.Sprintf("svc-%03d", i+1),
			TenantID:     "test-tenant",
			Name:         fmt.Sprintf("Test Service %d", i+1),
			Description:  fmt.Sprintf("Description for service %d", i+1),
			Price:        (i + 1) * 50_00,
			Currency:     "RUB",
			Duration:     fmt.Sprintf("%d min", 30+i*15),
			Provider:     providers[i%len(providers)],
			Availability: "available",
			Rating:       4.5,
			Images:       []string{fmt.Sprintf("https://img.test/svc-%d.jpg", i+1)},
		}
	}
	return services
}

// MockLLMClient implements ports.LLMPort returning pre-recorded responses.
type MockLLMClient struct {
	Responses []*domain.LLMResponse
	CallCount int
}

func NewMockLLMClient(responses ...*domain.LLMResponse) *MockLLMClient {
	return &MockLLMClient{Responses: responses}
}

func (m *MockLLMClient) Chat(_ context.Context, _ string) (string, error) {
	return "mock response", nil
}

func (m *MockLLMClient) ChatWithTools(_ context.Context, _ string, _ []domain.LLMMessage, _ []domain.ToolDefinition) (*domain.LLMResponse, error) {
	return m.next()
}

func (m *MockLLMClient) ChatWithToolsCached(_ context.Context, _ string, _ []domain.LLMMessage, _ []domain.ToolDefinition, _ *ports.CacheConfig) (*domain.LLMResponse, error) {
	return m.next()
}

func (m *MockLLMClient) ChatWithUsage(_ context.Context, _ string, _ string) (*ports.ChatResponse, error) {
	return &ports.ChatResponse{Text: "mock"}, nil
}

func (m *MockLLMClient) next() (*domain.LLMResponse, error) {
	idx := m.CallCount
	m.CallCount++
	if idx < len(m.Responses) {
		return m.Responses[idx], nil
	}
	return &domain.LLMResponse{
		Text:       "no more mock responses",
		StopReason: "end_turn",
	}, nil
}

// TestLogger returns a silent logger for tests.
func TestLogger() *logger.Logger {
	return logger.New("error")
}
