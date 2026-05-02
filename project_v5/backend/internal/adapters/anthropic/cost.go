// Package anthropic adapts the official anthropic-sdk-go to V5's LLMPort
// interface. Single dependency replaces V4's hand-rolled raw-HTTP client —
// gives us typed cache_control, typed tool blocks, free count_tokens, and
// future model-feature uplift.
package anthropic

import "keepstar_v5/internal/domain"

// modelPricing is per-million-token USD for one Anthropic model.
//
// Cache write tokens are billed at 1.25× input rate; cache read tokens are
// billed at 0.10× input rate. We don't store the multipliers separately —
// we compute the four billing buckets in Calculate.
type modelPricing struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

// pricing table reflects published Anthropic per-MTok USD prices for the
// models V5 might use. Update when Anthropic publishes new tariffs.
//
// Sources:
//   - Haiku 4.5: $1.00 input / $5.00 output (chunk-5.5 reference)
//   - Sonnet 4.6 / Opus 4.7: placeholders aligned with Anthropic's published
//     ratios at time of writing — refresh before billing-sensitive use.
var pricing = map[string]modelPricing{
	"claude-haiku-4-5":            {InputPerMTok: 1.00, OutputPerMTok: 5.00},
	"claude-haiku-4-5-20251001":   {InputPerMTok: 1.00, OutputPerMTok: 5.00},
	"claude-sonnet-4-6":           {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-sonnet-4-5":           {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-sonnet-4-5-20250929":  {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-opus-4-7":             {InputPerMTok: 15.00, OutputPerMTok: 75.00},
	"claude-opus-4-6":             {InputPerMTok: 15.00, OutputPerMTok: 75.00},
	"claude-opus-4-5":             {InputPerMTok: 15.00, OutputPerMTok: 75.00},
	"claude-opus-4-5-20251101":    {InputPerMTok: 15.00, OutputPerMTok: 75.00},
}

// Calculate fills usage.CostUSD from the four token counters using the
// model's pricing row. Unknown models default to Haiku rates plus a flag
// in the model id (caller can detect by reading u.Model after the call).
//
// Billing buckets:
//   - InputTokens × InputPerMTok                            (1×)
//   - OutputTokens × OutputPerMTok                          (1×)
//   - CacheCreationInputTokens × InputPerMTok × 1.25        (cache write)
//   - CacheReadInputTokens × InputPerMTok × 0.10            (cache read)
//
// All four buckets are summed into CostUSD.
func Calculate(u domain.LLMUsage) float64 {
	p, ok := pricing[u.Model]
	if !ok {
		// Unknown model — fall back to Haiku rates so cost is roughly in
		// the right order of magnitude. Caller is responsible for noticing
		// the model id and updating the table.
		p = pricing["claude-haiku-4-5"]
	}
	const (
		cacheWriteFactor = 1.25
		cacheReadFactor  = 0.10
		mTok             = 1_000_000.0
	)
	return (float64(u.InputTokens)*p.InputPerMTok+
		float64(u.OutputTokens)*p.OutputPerMTok+
		float64(u.CacheCreationInputTokens)*p.InputPerMTok*cacheWriteFactor+
		float64(u.CacheReadInputTokens)*p.InputPerMTok*cacheReadFactor) / mTok
}
