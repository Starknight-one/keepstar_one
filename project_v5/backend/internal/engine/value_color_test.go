package engine

import (
	"math"
	"testing"
)

func TestIsVariable(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want bool
	}{
		{"dollar prefixed string", "$foo", true},
		{"plain hex color", "#FFF", false},
		{"empty string", "", false},
		{"non-string number", 42, false},
		{"non-string nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsVariable(c.in); got != c.want {
				t.Fatalf("IsVariable(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestParseColor(t *testing.T) {
	const eps = 1e-9
	cases := []struct {
		name             string
		hex              string
		wantR, wantG, wantB, wantA float64
		wantErr          bool
	}{
		{"3-digit white", "#FFF", 1, 1, 1, 1, false},
		{"3-digit no hash", "ABC", 0xAA / 255.0, 0xBB / 255.0, 0xCC / 255.0, 1, false},
		{"6-digit black", "#000000", 0, 0, 0, 1, false},
		{"6-digit gray", "#808080", 0x80 / 255.0, 0x80 / 255.0, 0x80 / 255.0, 1, false},
		{"8-digit half alpha", "#80808080", 0x80 / 255.0, 0x80 / 255.0, 0x80 / 255.0, 0x80 / 255.0, false},
		{"invalid length 4", "#ABCD", 0, 0, 0, 0, true},
		{"invalid hex chars", "#GGGGGG", 0, 0, 0, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseColor(c.hex)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(got.R-c.wantR) > eps ||
				math.Abs(got.G-c.wantG) > eps ||
				math.Abs(got.B-c.wantB) > eps ||
				math.Abs(got.A-c.wantA) > eps {
				t.Fatalf("ParseColor(%q) = %+v, want (R=%v G=%v B=%v A=%v)", c.hex, got, c.wantR, c.wantG, c.wantB, c.wantA)
			}
		})
	}
}

func TestAsHelpers(t *testing.T) {
	if v, ok := AsBool(true); !ok || !v {
		t.Errorf("AsBool(true) = (%v, %v)", v, ok)
	}
	if _, ok := AsBool("true"); ok {
		t.Errorf("AsBool(\"true\") should be (_, false)")
	}
	if v, ok := AsNumber(3.14); !ok || v != 3.14 {
		t.Errorf("AsNumber(3.14) = (%v, %v)", v, ok)
	}
	if v, ok := AsNumber(42); !ok || v != 42 {
		t.Errorf("AsNumber(42) = (%v, %v)", v, ok)
	}
	if _, ok := AsNumber("3"); ok {
		t.Errorf("AsNumber(\"3\") should be (_, false)")
	}
	if v, ok := AsString("hello"); !ok || v != "hello" {
		t.Errorf("AsString(\"hello\") = (%q, %v)", v, ok)
	}
	if _, ok := AsString("$var"); ok {
		t.Errorf("AsString(\"$var\") should be (_, false)")
	}
}
