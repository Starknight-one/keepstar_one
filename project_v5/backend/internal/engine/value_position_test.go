package engine

import "testing"

func TestParseSizing(t *testing.T) {
	cases := []struct {
		name     string
		in       any
		wantMode SizingMode
		wantFB   float64
	}{
		{"nil", nil, SizingFixed, 0},
		{"int", 42, SizingFixed, 42},
		{"float", 3.5, SizingFixed, 3.5},
		{"fill_container", "fill_container", SizingFill, 0},
		{"fill_container with fallback", "fill_container(100)", SizingFill, 100},
		{"fit_content", "fit_content", SizingFit, 0},
		{"fit_content with fallback", "fit_content(200)", SizingFit, 200},
		{"unknown string", "auto", SizingFixed, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseSizing(c.in)
			if got.Mode != c.wantMode || got.Fallback != c.wantFB {
				t.Fatalf("ParseSizing(%v) = %+v, want mode=%v fallback=%v", c.in, got, c.wantMode, c.wantFB)
			}
		})
	}
}

func TestNormalizePadding(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want [4]float64
	}{
		{"nil", nil, [4]float64{0, 0, 0, 0}},
		{"single number", 8, [4]float64{8, 8, 8, 8}},
		{"variable string", "$pad", [4]float64{0, 0, 0, 0}},
		{"v,h", []any{4, 8}, [4]float64{4, 8, 4, 8}},
		{"trbl", []any{1, 2, 3, 4}, [4]float64{1, 2, 3, 4}},
		{"trbl with variable", []any{"$x", 2, 3, 4}, [4]float64{0, 2, 3, 4}},
		{"native float slice", []float64{1, 2, 3, 4}, [4]float64{1, 2, 3, 4}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizePadding(c.in)
			if got != c.want {
				t.Fatalf("NormalizePadding(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
