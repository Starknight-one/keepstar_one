package postgres

import "testing"

func TestFormatPrice(t *testing.T) {
	cases := []struct {
		name     string
		cents    int
		currency string
		want     string
	}{
		{"usd default for empty currency", 1499, "", "$14.99"},
		{"usd explicit", 1499, "USD", "$14.99"},
		{"usd thousands", 1234567, "USD", "$12,345.67"},
		{"usd zero fraction", 200000, "USD", "$2,000.00"},
		{"eur explicit", 1499, "EUR", "€14.99"},
		{"unknown currency falls back to code prefix", 1499, "GBP", "GBP 14.99"},
		{"sub-dollar", 5, "USD", "$0.05"},
		{"single digit dollars", 199, "USD", "$1.99"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := formatPrice(c.cents, c.currency)
			if got != c.want {
				t.Errorf("formatPrice(%d, %q) = %q, want %q", c.cents, c.currency, got, c.want)
			}
		})
	}
}
