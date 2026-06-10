package presets

// Navigation adjacency map: which preset to drill into when the user
// clicks a card rendered by preset X.
//
// Today the map is hardcoded against the system preset catalog. When
// the canvas microservice (Stream B) ships, tenants will draw the same
// "card → detail" edges directly in the v9 canvas and the adapter will
// surface them via v5_presets.metadata JSONB. The lookup signature
// stays the same — only the source moves.
//
// Why hardcoded for now: the only real edge in the V5 catalog today is
// product_card variants → product_detail variants. Categories /
// services land later, at which point this file gains 1-2 more rows
// or moves to DB. Until then a 4-line map is the cheapest correct
// implementation.
//
// Per the shop-layer / chat-layer split: the LLM never picks the
// drill target. It picks the visual shape (preset name); the
// adjacency table picks the next visual shape on user click. Stays
// here in backend, not in the chat-layer prompt.
var SystemAdjacency = map[string]string{
	"product_card":            "product_detail",
	"product_card_compact":    "product_detail",
	"product_card_horizontal": "product_detail_horizontal",
	"product_card_list_row":   "product_detail",
	"product_carousel":        "product_detail",
	"product_comparison":      "product_detail",
}

// AdjacentPreset returns the drill target for the given source preset
// (or "" if no edge is registered).
func AdjacentPreset(sourceName string) string {
	return SystemAdjacency[sourceName]
}
