package usecases

import (
	"context"
	"errors"
	"testing"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// fakeVariantsPort is a small in-memory MasterVariantsPort for cascade tests.
// Only Find* methods are populated; Upsert/Get/etc panic — they aren't part
// of the cascade contract and shouldn't be called.
type fakeVariantsPort struct {
	byGTIN          map[string][]domain.MasterVariant
	byVendorSKU     map[string][]domain.MasterVariant
	byVendorTitle   []ports.MasterVariantWithScore
	byEmbedding     []ports.MasterVariantWithScore
	embeddingErr    error
	gtinErr         error
}

func (f *fakeVariantsPort) UpsertMasterVariant(_ context.Context, _ *domain.MasterVariant) (string, error) {
	panic("not used in cascade tests")
}
func (f *fakeVariantsPort) GetMasterVariant(_ context.Context, _ string) (*domain.MasterVariant, error) {
	panic("not used in cascade tests")
}
func (f *fakeVariantsPort) ListMasterVariants(_ context.Context, _ string) ([]domain.MasterVariant, error) {
	panic("not used in cascade tests")
}
func (f *fakeVariantsPort) UpsertMasterCosmetics(_ context.Context, _ *domain.MasterCosmetics) error {
	panic("not used")
}
func (f *fakeVariantsPort) GetMasterCosmetics(_ context.Context, _ string) (*domain.MasterCosmetics, error) {
	panic("not used")
}

func (f *fakeVariantsPort) FindByGTIN(_ context.Context, gtins []string) ([]domain.MasterVariant, error) {
	if f.gtinErr != nil {
		return nil, f.gtinErr
	}
	for _, g := range gtins {
		if hits, ok := f.byGTIN[g]; ok {
			return hits, nil
		}
	}
	return nil, nil
}

func (f *fakeVariantsPort) FindByVendorAndSKU(_ context.Context, vendor, sku string) ([]domain.MasterVariant, error) {
	if hits, ok := f.byVendorSKU[vendor+":"+sku]; ok {
		return hits, nil
	}
	return nil, nil
}

func (f *fakeVariantsPort) FindByVendorAndFuzzyName(_ context.Context, _, _ string, _ float64, _ int) ([]ports.MasterVariantWithScore, error) {
	return f.byVendorTitle, nil
}

func (f *fakeVariantsPort) FindByEmbedding(_ context.Context, _ []float32, _ float64, _ int) ([]ports.MasterVariantWithScore, error) {
	if f.embeddingErr != nil {
		return nil, f.embeddingErr
	}
	return f.byEmbedding, nil
}

// Stubs for the discovery-agent methods — not exercised by cascade tests.
func (f *fakeVariantsPort) FindMasterProductsByEmbedding(_ context.Context, _ []float32, _ string, _ int) ([]ports.MasterProductSummary, error) {
	panic("not used in cascade tests")
}
func (f *fakeVariantsPort) FindMasterProductsByName(_ context.Context, _, _ string, _ int) ([]ports.MasterProductSummary, error) {
	panic("not used in cascade tests")
}
func (f *fakeVariantsPort) GetMasterProductSummary(_ context.Context, _ string) (*ports.MasterProductSummary, error) {
	panic("not used in cascade tests")
}

// fakeEmbedder always returns one fixed vector unless err is set.
type fakeEmbedder struct {
	err  error
	zero bool // when true returns []float32{} instead of a real vector
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i := range out {
		if f.zero {
			out[i] = []float32{}
		} else {
			out[i] = []float32{0.1, 0.2, 0.3}
		}
	}
	return out, nil
}

func newTestLogger() *logger.Logger {
	return logger.New("error")
}

func TestMatchCascade_GTINExact(t *testing.T) {
	port := &fakeVariantsPort{
		byGTIN: map[string][]domain.MasterVariant{
			"5901234123457": {{ID: "mv-1", MasterProductID: "mp-1"}},
		},
	}
	mc := NewMatchCascade(port, nil, newTestLogger())
	res, err := mc.Match(context.Background(), VariantInput{GTINs: []string{"5901234123457"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != domain.MatchOutcomeLinked {
		t.Fatalf("outcome=%s, want linked", res.Outcome)
	}
	if res.MasterVariantID != "mv-1" {
		t.Fatalf("mvID=%s, want mv-1", res.MasterVariantID)
	}
	if res.Confidence != domain.MatchConfidenceGTINExact {
		t.Fatalf("conf=%s, want gtin_exact", res.Confidence)
	}
}

func TestMatchCascade_GTINCollision_GoesToReview(t *testing.T) {
	port := &fakeVariantsPort{
		byGTIN: map[string][]domain.MasterVariant{
			"5901234123457": {
				{ID: "mv-1"},
				{ID: "mv-2"},
			},
		},
	}
	mc := NewMatchCascade(port, nil, newTestLogger())
	res, err := mc.Match(context.Background(), VariantInput{GTINs: []string{"5901234123457"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != domain.MatchOutcomeReviewQueue {
		t.Fatalf("outcome=%s, want review_queue", res.Outcome)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("candidates=%d, want 2", len(res.Candidates))
	}
}

func TestMatchCascade_VendorAndSKU(t *testing.T) {
	port := &fakeVariantsPort{
		byVendorSKU: map[string][]domain.MasterVariant{
			"Dr.Althea:DA-001": {{ID: "mv-3"}},
		},
	}
	mc := NewMatchCascade(port, nil, newTestLogger())
	res, err := mc.Match(context.Background(), VariantInput{
		Vendor: "Dr.Althea",
		SKU:    "DA-001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != domain.MatchOutcomeLinked {
		t.Fatalf("outcome=%s", res.Outcome)
	}
	if res.Confidence != domain.MatchConfidenceSKUExact {
		t.Fatalf("conf=%s", res.Confidence)
	}
}

func TestMatchCascade_FuzzyTitle_AxesMatch(t *testing.T) {
	port := &fakeVariantsPort{
		byVendorTitle: []ports.MasterVariantWithScore{
			{
				MasterVariant: domain.MasterVariant{
					ID:   "mv-fuzzy",
					Axes: map[string]string{"Size": "236ml"},
				},
				MatchedScore: 0.91,
			},
		},
	}
	mc := NewMatchCascade(port, nil, newTestLogger())
	res, err := mc.Match(context.Background(), VariantInput{
		Vendor:       "Dr.Althea",
		ProductTitle: "Hydrating Cleanser",
		Axes:         map[string]string{"size": "236ml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != domain.MatchOutcomeLinked {
		t.Fatalf("outcome=%s, want linked", res.Outcome)
	}
	if res.Confidence != domain.MatchConfidenceFuzzyHigh {
		t.Fatalf("conf=%s", res.Confidence)
	}
}

func TestMatchCascade_FuzzyTitle_AxesMismatch_FallsThrough(t *testing.T) {
	port := &fakeVariantsPort{
		byVendorTitle: []ports.MasterVariantWithScore{
			{
				MasterVariant: domain.MasterVariant{
					ID:   "mv-x",
					Axes: map[string]string{"Size": "473ml"},
				},
				MatchedScore: 0.95,
			},
		},
	}
	mc := NewMatchCascade(port, nil, newTestLogger())
	res, err := mc.Match(context.Background(), VariantInput{
		Vendor:       "Dr.Althea",
		ProductTitle: "Hydrating Cleanser",
		Axes:         map[string]string{"size": "236ml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != domain.MatchOutcomeNewMaster {
		t.Fatalf("outcome=%s, want new_master", res.Outcome)
	}
}

func TestMatchCascade_EmbeddingMatch_GoesToReview(t *testing.T) {
	port := &fakeVariantsPort{
		byEmbedding: []ports.MasterVariantWithScore{
			{MasterVariant: domain.MasterVariant{ID: "mv-emb-1"}, MatchedScore: 0.94},
			{MasterVariant: domain.MasterVariant{ID: "mv-emb-2"}, MatchedScore: 0.93},
		},
	}
	mc := NewMatchCascade(port, &fakeEmbedder{}, newTestLogger())
	res, err := mc.Match(context.Background(), VariantInput{
		ProductTitle: "Some Product",
		Brand:        "BrandX",
		Description:  "rich text",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != domain.MatchOutcomeReviewQueue {
		t.Fatalf("outcome=%s, want review_queue", res.Outcome)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("candidates=%d, want 2", len(res.Candidates))
	}
}

func TestMatchCascade_NoMatch_NewMaster(t *testing.T) {
	port := &fakeVariantsPort{}
	mc := NewMatchCascade(port, &fakeEmbedder{}, newTestLogger())
	res, err := mc.Match(context.Background(), VariantInput{
		ProductTitle: "Brand New Thing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != domain.MatchOutcomeNewMaster {
		t.Fatalf("outcome=%s, want new_master", res.Outcome)
	}
	if res.Confidence != domain.MatchConfidenceUnverified {
		t.Fatalf("conf=%s", res.Confidence)
	}
}

func TestMatchCascade_EmbeddingFailure_FallsThrough(t *testing.T) {
	port := &fakeVariantsPort{}
	mc := NewMatchCascade(port, &fakeEmbedder{err: errors.New("openai down")}, newTestLogger())
	res, err := mc.Match(context.Background(), VariantInput{
		ProductTitle: "Brand New Thing",
		Brand:        "X",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != domain.MatchOutcomeNewMaster {
		t.Fatalf("outcome=%s, want new_master (graceful degradation)", res.Outcome)
	}
}

func TestMatchCascade_GTINLookupError_PropagatesUp(t *testing.T) {
	port := &fakeVariantsPort{gtinErr: errors.New("db down")}
	mc := NewMatchCascade(port, nil, newTestLogger())
	_, err := mc.Match(context.Background(), VariantInput{GTINs: []string{"x"}})
	if err == nil {
		t.Fatal("expected error from gtin lookup")
	}
}

func TestAxesMatch(t *testing.T) {
	cases := []struct {
		name      string
		input     map[string]string
		candidate map[string]string
		want      bool
	}{
		{"empty input matches anything", nil, map[string]string{"x": "y"}, true},
		{"empty candidate doesn't match non-empty input", map[string]string{"x": "y"}, nil, false},
		{"exact match", map[string]string{"size": "236ml"}, map[string]string{"size": "236ml"}, true},
		{"case-insensitive keys + values", map[string]string{"Size": "236ML"}, map[string]string{"size": "236ml"}, true},
		{"missing key in candidate", map[string]string{"size": "236ml", "color": "red"}, map[string]string{"size": "236ml"}, false},
		{"value mismatch", map[string]string{"size": "236ml"}, map[string]string{"size": "473ml"}, false},
		{"extra axes on candidate ok", map[string]string{"size": "236ml"}, map[string]string{"size": "236ml", "color": "red"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := axesMatch(tc.input, tc.candidate); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}
