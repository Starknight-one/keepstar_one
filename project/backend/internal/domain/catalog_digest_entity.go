package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// CatalogDigest is a pre-computed meta-schema of a tenant's catalog.
// It tells Agent1 HOW to search (strategy), not WHAT exists (data dump).
type CatalogDigest struct {
	GeneratedAt   time.Time        `json:"generated_at"`
	TotalProducts int              `json:"total_products"`
	Categories    []DigestCategory `json:"categories"`
}

// DigestCategory describes one product category in the digest.
type DigestCategory struct {
	Name       string        `json:"name"`
	Slug       string        `json:"slug"`
	Count      int           `json:"count"`
	PriceRange [2]int        `json:"price_range"` // kopecks [min, max]
	Params     []DigestParam `json:"params"`
}

// DigestParam describes a single filterable parameter within a category.
type DigestParam struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`                    // "enum" | "range"
	Cardinality int      `json:"cardinality"`
	Values      []string `json:"values,omitempty"`   // full list (cardinality <= 15)
	Top         []string `json:"top,omitempty"`      // top-N (cardinality 16-50)
	More        int      `json:"more,omitempty"`     // remaining count (cardinality 16-50)
	Families    []string `json:"families,omitempty"` // families (cardinality 50+)
	Range       string   `json:"range,omitempty"`    // "36-47", "13-17 inch"
}

// ToPromptText returns compact text for LLM system prompt.
// Includes search strategy hints (→ filter / → vector_query).
func (d *CatalogDigest) ToPromptText() string {
	if d == nil || len(d.Categories) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Tenant catalog: %d products\n\n", d.TotalProducts))

	// Determine how many categories to show
	maxCategories := 25
	cats := d.Categories
	remaining := 0
	if len(cats) > maxCategories {
		remaining = len(cats) - maxCategories
		cats = cats[:maxCategories]
	}

	for _, cat := range cats {
		// Category line: Name (count): price range
		priceStr := formatPriceRange(cat.PriceRange[0], cat.PriceRange[1])
		b.WriteString(fmt.Sprintf("%s (%d): %s\n", cat.Name, cat.Count, priceStr))

		// Params
		for _, p := range cat.Params {
			b.WriteString("  ")
			b.WriteString(formatParam(p))
			b.WriteString("\n")
		}
	}

	if remaining > 0 {
		b.WriteString(fmt.Sprintf("\n...and %d more categories\n", remaining))
	}

	// Search strategy block
	b.WriteString("\nSearch strategy:\n")
	b.WriteString("- param marked \"→ filter\": use in filters.{param} (exact SQL match)\n")
	b.WriteString("- param marked \"→ vector_query\": include in vector_query text (semantic match)\n")
	b.WriteString("- broad/activity queries (\"для бега\", \"в подарок\"): do NOT set category filter, use only vector_query + price\n")
	b.WriteString("- if unsure about category: omit category filter, let vector search find across all categories\n")

	return b.String()
}

// formatPriceRange converts kopecks to rubles for display.
// If min == max or min == 0: "from N RUB". Otherwise "N-M RUB".
func formatPriceRange(minKop, maxKop int) string {
	minRub := minKop / 100
	maxRub := maxKop / 100
	if minRub == 0 && maxRub == 0 {
		return "price N/A"
	}
	if minRub == maxRub || minRub == 0 {
		return fmt.Sprintf("from %d RUB", maxRub)
	}
	if maxRub == 0 {
		return fmt.Sprintf("from %d RUB", minRub)
	}
	return fmt.Sprintf("%d-%d RUB", minRub, maxRub)
}

// formatParam formats a single DigestParam with search hint.
func formatParam(p DigestParam) string {
	hint := "→ filter"

	switch {
	case p.Range != "":
		return fmt.Sprintf("%s(%s: %s) %s", p.Key, p.Type, p.Range, hint)
	case len(p.Values) > 0:
		return fmt.Sprintf("%s(%d): %s %s", p.Key, p.Cardinality, strings.Join(p.Values, ", "), hint)
	case len(p.Top) > 0:
		return fmt.Sprintf("%s(%d, top: %s +%d) %s", p.Key, p.Cardinality, strings.Join(p.Top, "/"), p.More, hint)
	case len(p.Families) > 0:
		hint = "→ vector_query"
		return fmt.Sprintf("%s(%d, families: %s) %s", p.Key, p.Cardinality, strings.Join(p.Families, "/"), hint)
	default:
		return fmt.Sprintf("%s(%d) %s", p.Key, p.Cardinality, hint)
	}
}

// colorFamilyMap maps color names (Russian/English) to color families.
var colorFamilyMap = map[string]string{
	// Red family
	"red": "Red", "красный": "Red", "бордовый": "Red", "алый": "Red",
	"малиновый": "Red", "вишнёвый": "Red", "вишневый": "Red", "рубиновый": "Red",
	"коралловый": "Red", "багровый": "Red", "crimson": "Red", "maroon": "Red",
	"burgundy": "Red", "cherry": "Red", "coral": "Red", "wine": "Red",

	// Blue family
	"blue": "Blue", "синий": "Blue", "голубой": "Blue", "тёмно-синий": "Blue",
	"темно-синий": "Blue", "лазурный": "Blue", "бирюзовый": "Blue", "индиго": "Blue",
	"navy": "Blue", "azure": "Blue", "cobalt": "Blue", "turquoise": "Blue",
	"teal": "Blue", "cyan": "Blue", "sky blue": "Blue", "royal blue": "Blue",

	// Green family
	"green": "Green", "зелёный": "Green", "зеленый": "Green", "салатовый": "Green",
	"травяной": "Green", "нежно-травяной": "Green", "изумрудный": "Green", "оливковый": "Green",
	"хаки": "Green", "мятный": "Green", "lime": "Green", "olive": "Green",
	"emerald": "Green", "mint": "Green", "forest green": "Green", "sage": "Green",
	"khaki": "Green",

	// Black family
	"black": "Black", "чёрный": "Black", "черный": "Black", "графитовый": "Black",
	"угольный": "Black", "антрацит": "Black", "graphite": "Black", "charcoal": "Black",
	"onyx": "Black", "jet black": "Black",

	// White family
	"white": "White", "белый": "White", "молочный": "White", "кремовый": "White",
	"слоновая кость": "White", "ivory": "White", "cream": "White", "snow": "White",
	"pearl": "White", "off-white": "White",

	// Gray family
	"gray": "Gray", "grey": "Gray", "серый": "Gray", "серебристый": "Gray",
	"стальной": "Gray", "silver": "Gray", "steel": "Gray", "platinum": "Gray",
	"ash": "Gray", "slate": "Gray",

	// Yellow family
	"yellow": "Yellow", "жёлтый": "Yellow", "желтый": "Yellow", "золотой": "Yellow",
	"лимонный": "Yellow", "янтарный": "Yellow", "gold": "Yellow", "lemon": "Yellow",
	"amber": "Yellow", "mustard": "Yellow",

	// Orange family
	"orange": "Orange", "оранжевый": "Orange", "рыжий": "Orange", "апельсиновый": "Orange",
	"мандариновый": "Orange", "tangerine": "Orange", "peach": "Orange", "персиковый": "Orange",

	// Pink family
	"pink": "Pink", "розовый": "Pink", "фуксия": "Pink", "пурпурный": "Pink",
	"magenta": "Pink", "fuchsia": "Pink", "hot pink": "Pink", "salmon": "Pink",
	"rose": "Pink", "blush": "Pink",

	// Purple family
	"purple": "Purple", "фиолетовый": "Purple", "лиловый": "Purple", "сиреневый": "Purple",
	"лавандовый": "Purple", "violet": "Purple", "lavender": "Purple", "plum": "Purple",
	"mauve": "Purple", "lilac": "Purple",

	// Brown family
	"brown": "Brown", "коричневый": "Brown", "шоколадный": "Brown", "каштановый": "Brown",
	"бежевый": "Brown", "песочный": "Brown", "beige": "Brown", "tan": "Brown",
	"chocolate": "Brown", "sand": "Brown", "camel": "Brown", "taupe": "Brown",
}

// ComputeFamilies groups a list of color values into color families.
// For non-color attributes, returns top-10 most frequent values.
func ComputeFamilies(key string, values []string) []string {
	isColor := strings.EqualFold(key, "color") || strings.EqualFold(key, "цвет")

	if isColor {
		return computeColorFamilies(values)
	}

	// For non-color: return top-10
	if len(values) <= 10 {
		return values
	}
	return values[:10]
}

// computeColorFamilies maps raw color values to color family names.
func computeColorFamilies(values []string) []string {
	familySet := make(map[string]bool)
	for _, v := range values {
		lower := strings.ToLower(strings.TrimSpace(v))
		if family, ok := colorFamilyMap[lower]; ok {
			familySet[family] = true
		}
	}

	// Sort families for deterministic output
	families := make([]string, 0, len(familySet))
	for f := range familySet {
		families = append(families, f)
	}
	sort.Strings(families)
	return families
}
