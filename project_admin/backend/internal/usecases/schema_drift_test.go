package usecases

import (
	"context"
	"encoding/json"
	"testing"

	"keepstar-admin/internal/logger"
)

// fakeBroker captures Publish calls for assertions. Doesn't actually
// connect to anything.
type fakeBroker struct {
	published []publishedMsg
	failNext  error
}

type publishedMsg struct {
	topic   string
	payload []byte
}

func (f *fakeBroker) Publish(_ context.Context, topic string, payload []byte) error {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}
	f.published = append(f.published, publishedMsg{topic: topic, payload: append([]byte(nil), payload...)})
	return nil
}

func (f *fakeBroker) Subscribe(_ context.Context, _, _, _ string, _ func(context.Context, []byte) error) error {
	return nil // not exercised in producer tests
}

func (f *fakeBroker) Close() error { return nil }

func TestSchemaDrift_PublishJob_PutsTenantAndRunIntoStream(t *testing.T) {
	fb := &fakeBroker{}
	uc := &SchemaDriftUseCase{
		broker: fb,
		log:    logger.New("error"),
	}
	if err := uc.PublishJob(context.Background(), "t-abc", "apply-123-feedf00d"); err != nil {
		t.Fatalf("PublishJob: %v", err)
	}
	if len(fb.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(fb.published))
	}
	msg := fb.published[0]
	if msg.topic != DriftTopic() {
		t.Errorf("topic = %q, want %q", msg.topic, DriftTopic())
	}
	var job DriftJob
	if err := json.Unmarshal(msg.payload, &job); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if job.TenantID != "t-abc" || job.ApplyRunID != "apply-123-feedf00d" {
		t.Errorf("job = %+v, want tenant=t-abc run=apply-123-feedf00d", job)
	}
}

func TestSchemaDrift_PublishJob_NilBrokerIsNoOp(t *testing.T) {
	uc := &SchemaDriftUseCase{
		broker: nil,
		log:    logger.New("error"),
	}
	// Should not panic, should not error.
	if err := uc.PublishJob(context.Background(), "t-1", "run-1"); err != nil {
		t.Errorf("nil-broker publish should be a no-op, got %v", err)
	}
}

func TestSchemaDrift_PublishJob_PropagatesBrokerError(t *testing.T) {
	fb := &fakeBroker{failNext: errBrokerDown}
	uc := &SchemaDriftUseCase{
		broker: fb,
		log:    logger.New("error"),
	}
	err := uc.PublishJob(context.Background(), "t-1", "run-1")
	if err == nil {
		t.Fatal("expected error from broker failure")
	}
}

// sentinel error used by the failure test.
var errBrokerDown = &brokerDownError{}

type brokerDownError struct{}

func (*brokerDownError) Error() string { return "broker down" }
