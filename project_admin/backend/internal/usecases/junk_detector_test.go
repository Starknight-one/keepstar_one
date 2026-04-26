package usecases

import (
	"testing"

	"keepstar-admin/internal/domain"
)

func TestDetectJunk(t *testing.T) {
	cases := []struct {
		name    string
		in      JunkDetectorInput
		wantSet map[domain.JunkSignal]bool
		wantJunk bool
	}{
		{
			name: "real product — no signals",
			in: JunkDetectorInput{
				AxisName: "236ml", HasGTIN: true, HasSKU: true,
				HasWeight: true, HasVolume: true,
				PriceCents: 2999, ParentMinPrice: 2999,
			},
			wantSet:  map[domain.JunkSignal]bool{},
			wantJunk: false,
		},
		{
			name: "classic gift wrap — all four signals",
			in: JunkDetectorInput{
				AxisName: "Gift wrap", HasGTIN: false, HasSKU: false,
				HasWeight: false, HasVolume: false,
				PriceCents: 500, ParentMinPrice: 2999,
			},
			wantSet: map[domain.JunkSignal]bool{
				domain.JunkSignalAxisNamePattern: true,
				domain.JunkSignalNoIdentifiers:   true,
				domain.JunkSignalNoDimensions:    true,
				domain.JunkSignalSmallPriceDelta: true,
			},
			wantJunk: true,
		},
		{
			name: "engraving without identifiers — 3 signals",
			in: JunkDetectorInput{
				AxisName: "Custom engraving", HasGTIN: false, HasSKU: false,
				HasWeight: false, HasVolume: false,
				PriceCents: 1500, ParentMinPrice: 0,
			},
			wantSet: map[domain.JunkSignal]bool{
				domain.JunkSignalAxisNamePattern: true,
				domain.JunkSignalNoIdentifiers:   true,
				domain.JunkSignalNoDimensions:    true,
			},
			wantJunk: true,
		},
		{
			name: "real product missing dimensions only — 1 signal, not junk",
			in: JunkDetectorInput{
				AxisName: "236ml", HasGTIN: true, HasSKU: true,
				HasWeight: false, HasVolume: false,
				PriceCents: 2999, ParentMinPrice: 2999,
			},
			wantSet: map[domain.JunkSignal]bool{
				domain.JunkSignalNoDimensions: true,
			},
			wantJunk: false,
		},
		{
			name: "extended warranty — axis pattern, no identifiers, no dims",
			in: JunkDetectorInput{
				AxisName: "Extended Warranty 2 years", HasGTIN: false, HasSKU: false,
				HasWeight: false, HasVolume: false,
				PriceCents: 1999, ParentMinPrice: 99900,
			},
			wantSet: map[domain.JunkSignal]bool{
				domain.JunkSignalAxisNamePattern: true,
				domain.JunkSignalNoIdentifiers:   true,
				domain.JunkSignalNoDimensions:    true,
				// price $19.99 > $10 threshold → small_price_delta NOT firing
			},
			wantJunk: true,
		},
		{
			name: "addon with hyphen — pattern matches",
			in: JunkDetectorInput{
				AxisName: "Add-on bag", HasGTIN: false, HasSKU: false,
				PriceCents: 500, ParentMinPrice: 4900,
			},
			wantSet: map[domain.JunkSignal]bool{
				domain.JunkSignalAxisNamePattern: true,
				domain.JunkSignalNoIdentifiers:   true,
				domain.JunkSignalNoDimensions:    true,
				domain.JunkSignalSmallPriceDelta: true,
			},
			wantJunk: true,
		},
		{
			name: "warranty bag — false positive risk that should NOT trigger",
			// "warranty bag" is a real product — NOT junk. But our regex has
			// \b(warranty)\b so this WILL match. Document the limitation:
			// merchants who sell "Warranty bag for X" can mark this false
			// positive in the Detected Add-ons UI (M9). Test asserts the
			// current behavior so we notice if regex tightens later.
			in: JunkDetectorInput{
				AxisName: "Warranty bag XL", HasGTIN: true, HasSKU: true,
				HasWeight: true, HasVolume: true,
				PriceCents: 4900, ParentMinPrice: 4900,
			},
			wantSet: map[domain.JunkSignal]bool{
				domain.JunkSignalAxisNamePattern: true,
			},
			// Only 1 signal because identifiers + dims present, price > $10.
			wantJunk: false,
		},
		{
			name: "small expensive watch variant — single signal isn't junk",
			in: JunkDetectorInput{
				AxisName: "44mm Black Strap", HasGTIN: true, HasSKU: true,
				HasWeight: true, HasVolume: false,
				PriceCents: 39900, ParentMinPrice: 39900,
			},
			wantSet:  map[domain.JunkSignal]bool{},
			wantJunk: false,
		},
		{
			name: "sample size — no identifiers + small absolute price = 2 signals",
			in: JunkDetectorInput{
				AxisName: "Sample size", HasGTIN: false, HasSKU: false,
				HasWeight: true, HasVolume: true,
				PriceCents: 200, ParentMinPrice: 200,
			},
			wantSet: map[domain.JunkSignal]bool{
				domain.JunkSignalNoIdentifiers:   true,
				domain.JunkSignalSmallPriceDelta: true,
			},
			wantJunk: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectJunk(tc.in)
			gotSet := map[domain.JunkSignal]bool{}
			for _, s := range got {
				gotSet[s] = true
			}
			for s := range tc.wantSet {
				if !gotSet[s] {
					t.Errorf("missing expected signal %q (got %v)", s, got)
				}
			}
			for s := range gotSet {
				if !tc.wantSet[s] {
					t.Errorf("unexpected signal %q (got %v)", s, got)
				}
			}
			if IsJunk(tc.in) != tc.wantJunk {
				t.Errorf("IsJunk = %v, want %v (signals: %v)", !tc.wantJunk, tc.wantJunk, got)
			}
		})
	}
}

func TestSignalReasonMap(t *testing.T) {
	signals := []domain.JunkSignal{
		domain.JunkSignalAxisNamePattern,
		domain.JunkSignalNoDimensions,
	}
	m := SignalReasonMap(signals, "Gift wrap (red)")
	if got := m[string(domain.JunkSignalAxisNamePattern)]; got != "Gift wrap (red)" {
		t.Errorf("axis reason = %q, want %q", got, "Gift wrap (red)")
	}
	if _, ok := m[string(domain.JunkSignalNoDimensions)]; !ok {
		t.Errorf("missing no_dimensions reason")
	}
	if _, ok := m[string(domain.JunkSignalNoIdentifiers)]; ok {
		t.Errorf("unexpected no_identifiers reason")
	}
}
