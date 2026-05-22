package usecases

import (
	"testing"

	"keepstar-admin/internal/domain"
)

func TestGuessFieldType_Numeric(t *testing.T) {
	s := &domain.InboxFieldStats{
		NonNullCount:  100,
		DistinctCount: 87,
		SampleValues:  []string{"1.99", "2.50", "100", "0.5"},
	}
	if got := GuessFieldType(s); got != "numeric" {
		t.Errorf("got %q, want numeric", got)
	}
}

func TestGuessFieldType_Categorical(t *testing.T) {
	s := &domain.InboxFieldStats{
		NonNullCount:  1000,
		DistinctCount: 8,
		SampleValues:  []string{"cosmetics", "haircare", "skincare", "makeup"},
	}
	if got := GuessFieldType(s); got != "categorical" {
		t.Errorf("got %q, want categorical (distinct=8 of 1000 — low-cardinality)", got)
	}
}

func TestGuessFieldType_Text(t *testing.T) {
	s := &domain.InboxFieldStats{
		NonNullCount:  1000,
		DistinctCount: 950,
		SampleValues:  []string{"Hyaluronic Cream 50ml", "Vitamin C Serum"},
	}
	if got := GuessFieldType(s); got != "text" {
		t.Errorf("got %q, want text (distinct=950 of 1000 — high-cardinality)", got)
	}
}

func TestGuessFieldType_Date(t *testing.T) {
	s := &domain.InboxFieldStats{
		NonNullCount:  50,
		DistinctCount: 50,
		SampleValues:  []string{"2026-05-22", "2026-05-21", "2025-12-31"},
	}
	if got := GuessFieldType(s); got != "date" {
		t.Errorf("got %q, want date", got)
	}
}

func TestGuessFieldType_DateISORFC3339(t *testing.T) {
	s := &domain.InboxFieldStats{
		NonNullCount:  10,
		DistinctCount: 10,
		SampleValues:  []string{"2026-05-22T10:15:00Z", "2025-01-01T00:00:00.123+03:00"},
	}
	if got := GuessFieldType(s); got != "date" {
		t.Errorf("got %q, want date for RFC3339-ish samples", got)
	}
}

func TestGuessFieldType_EmptyStats(t *testing.T) {
	if got := GuessFieldType(nil); got != "unknown" {
		t.Errorf("nil stats: got %q, want unknown", got)
	}
	if got := GuessFieldType(&domain.InboxFieldStats{NonNullCount: 0}); got != "unknown" {
		t.Errorf("zero-count stats: got %q, want unknown", got)
	}
}

func TestGuessFieldType_NumericWithBlanks(t *testing.T) {
	// Blank samples should be ignored — if remaining all numeric, still numeric.
	s := &domain.InboxFieldStats{
		NonNullCount:  50,
		DistinctCount: 30,
		SampleValues:  []string{"", "1.5", "  ", "2.0", "3"},
	}
	if got := GuessFieldType(s); got != "numeric" {
		t.Errorf("got %q, want numeric (blanks should be ignored)", got)
	}
}

func TestGuessFieldType_CategoricalRatioBoundary(t *testing.T) {
	// distinct=15 of 100 → ratio 0.15 < 0.3 → categorical.
	s := &domain.InboxFieldStats{
		NonNullCount:  100,
		DistinctCount: 15,
		SampleValues:  []string{"alpha", "beta", "gamma"},
	}
	if got := GuessFieldType(s); got != "categorical" {
		t.Errorf("got %q, want categorical (ratio 0.15)", got)
	}
}

func TestGuessFieldType_TextWhenRatioTooHigh(t *testing.T) {
	// distinct=40 of 100 → ratio 0.40 > 0.3 → not categorical → text.
	s := &domain.InboxFieldStats{
		NonNullCount:  100,
		DistinctCount: 40,
		SampleValues:  []string{"some descriptive text here", "another long one"},
	}
	if got := GuessFieldType(s); got != "text" {
		t.Errorf("got %q, want text (ratio 0.40 too high for categorical)", got)
	}
}
