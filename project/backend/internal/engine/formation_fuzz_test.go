package engine

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"keepstar/internal/domain"
)

// =============================================================================
// Property-based tests for the visual assembly pipeline.
//
// Strategy:
//   1. Exhaustive on structural axes (count x layout x size x direction x show x hide)
//      = 20 x 5 x 5 x 3 x 15 x 14 = 315,000 combinations
//   2. Exhaustive on per-atom transforms (display x format x field)
//      = 23 x 8 x 13 = 2,392 combinations
//   3. Random fuzz for combined overrides (10,000 iterations)
//
// Every combination must satisfy ALL invariants.
// =============================================================================

// --- Parameter pools ---

var (
	fuzzLayouts    = []string{"", "grid", "list", "single", "carousel"}
	fuzzSizes      = []string{"", "tiny", "small", "medium", "large"}
	fuzzDirections = []string{"", "vertical", "horizontal"}
	fuzzFields     = []string{
		"images", "name", "price", "rating", "brand", "category",
		"description", "tags", "stockQuantity",
		"productForm", "skinType", "concern", "keyIngredients",
	}
	fuzzDisplays = []string{
		"h1", "h2", "h3", "h4",
		"body-lg", "body", "body-sm", "caption",
		"badge", "badge-success", "badge-error", "badge-warning",
		"tag", "tag-active",
		"price", "price-lg", "price-old", "price-discount",
		"rating", "rating-text", "rating-compact",
		"image-cover", "thumbnail",
	}
	fuzzFormats = []string{
		"currency", "stars", "stars-text", "stars-compact",
		"percent", "number", "date", "text",
	}
	fuzzColors = []string{"green", "red", "blue", "orange", "purple", "#FF0000", "#abc"}
	fuzzCounts = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

	// Show field options: empty + each individual field + "all"
	fuzzShowOptions = func() [][]string {
		opts := [][]string{nil} // empty = no explicit show
		for _, f := range fuzzFields {
			opts = append(opts, []string{f})
		}
		opts = append(opts, fuzzFields) // all fields
		return opts
	}()
	// Hide field options: empty + each individual field
	fuzzHideOptions = func() [][]string {
		opts := [][]string{nil} // empty = no hide
		for _, f := range fuzzFields {
			opts = append(opts, []string{f})
		}
		return opts
	}()
)

// --- Test parameter struct ---

type fuzzParams struct {
	Count            int
	Layout           string
	Size             string
	Direction        string
	ShowFields       []string
	HideFields       []string
	OrderFields      []string
	DisplayOverrides map[string]string
	FormatOverrides  map[string]string
	ColorMap         map[string]string
}

func (p fuzzParams) String() string {
	return fmt.Sprintf("count=%d layout=%q size=%q dir=%q show=%v hide=%v order=%v display=%v format=%v color=%v",
		p.Count, p.Layout, p.Size, p.Direction,
		p.ShowFields, p.HideFields, p.OrderFields,
		p.DisplayOverrides, p.FormatOverrides, p.ColorMap)
}

// --- Product generators ---

// Rich products with varying data completeness
func generateFuzzProducts(rng *rand.Rand, count int) []domain.Product {
	names := []string{
		"Hydrating Serum", "Glow Cream", "Eye Contour", "Lip Balm",
		"Sunscreen SPF50", "Night Repair Mask", "Vitamin C Booster",
		"Micellar Water", "Retinol Serum", "Peptide Moisturizer",
		"Clay Mask", "Toner Essence", "BB Cream", "Face Oil",
		"Exfoliating Scrub", "Collagen Serum", "Aloe Vera Gel",
		"Tea Tree Spot Treatment", "Niacinamide Serum", "Hyaluronic Acid",
	}
	products := make([]domain.Product, count)
	for i := 0; i < count; i++ {
		p := domain.Product{
			ID:       fmt.Sprintf("fuzz-%d", i),
			Name:     names[rng.Intn(len(names))],
			Currency: "$",
		}
		if rng.Float64() < 0.8 {
			p.Price = rng.Intn(50000) + 500
		}
		if rng.Float64() < 0.5 {
			p.Rating = float64(rng.Intn(50)) / 10.0
		}
		if rng.Float64() < 0.7 {
			p.Brand = "TestBrand"
		}
		if rng.Float64() < 0.6 {
			p.Category = "Essences"
		}
		if rng.Float64() < 0.4 {
			p.Description = "A wonderful product for your skin care routine."
		}
		if rng.Float64() < 0.6 {
			p.Images = []string{fmt.Sprintf("https://example.com/img/%d.jpg", i)}
		}
		if rng.Float64() < 0.3 {
			p.Tags = []string{"new", "bestseller"}
		}
		if rng.Float64() < 0.2 {
			p.StockQuantity = rng.Intn(100)
		}
		if rng.Float64() < 0.4 {
			p.ProductForm = "cream"
		}
		if rng.Float64() < 0.3 {
			p.SkinType = []string{"oily", "combination"}
		}
		if rng.Float64() < 0.3 {
			p.Concern = []string{"acne", "pores"}
		}
		if rng.Float64() < 0.3 {
			p.KeyIngredients = []string{"hyaluronic acid", "niacinamide"}
		}
		products[i] = p
	}
	return products
}

// --- Pipeline runner (mirrors testbench handler logic) ---

func runPipeline(params fuzzParams, products []domain.Product) ([]domain.Widget, domain.FormationType) {
	entityCount := len(products)
	resolved := AutoResolve("product", entityCount)
	fields := resolved.Fields
	layout := resolved.Layout
	size := resolved.Size

	if params.Layout != "" {
		layout = params.Layout
	}
	if params.Size != "" {
		size = domain.WidgetSize(params.Size)
	}

	hasExplicitShow := len(params.ShowFields) > 0
	for _, s := range params.ShowFields {
		found := false
		for _, f := range fields {
			if f == s {
				found = true
				break
			}
		}
		if !found {
			fields = append(fields, s)
		}
	}

	if len(params.HideFields) > 0 {
		hideSet := make(map[string]bool)
		for _, h := range params.HideFields {
			hideSet[h] = true
		}
		filtered := make([]string, 0)
		for _, f := range fields {
			if !hideSet[f] {
				filtered = append(filtered, f)
			}
		}
		fields = filtered
	}

	if len(params.OrderFields) > 0 {
		ordered := make([]string, 0, len(fields))
		inOrder := make(map[string]bool)
		for _, o := range params.OrderFields {
			for _, f := range fields {
				if f == o && !inOrder[o] {
					ordered = append(ordered, o)
					inOrder[o] = true
					break
				}
			}
		}
		for _, f := range fields {
			if !inOrder[f] {
				ordered = append(ordered, f)
			}
		}
		fields = ordered
	}

	fieldConfigs := BuildFieldConfigsWithFormat(fields, params.DisplayOverrides, params.FormatOverrides)

	var formationMode domain.FormationType
	switch layout {
	case "grid":
		formationMode = domain.FormationTypeGrid
	case "list":
		formationMode = domain.FormationTypeList
	case "carousel":
		formationMode = domain.FormationTypeCarousel
	case "single":
		formationMode = domain.FormationTypeSingle
	default:
		formationMode = domain.FormationTypeGrid
	}

	widgets := BuildVisualWidgets(fieldConfigs, "GenericCard", size, entityCount, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		p := products[i]
		return ProductFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
	}, domain.EntityTypeProduct)

	for i := range widgets {
		widgets[i].Atoms = ApplyAtomConstraints(widgets[i].Atoms)
		ApplyWidgetConstraints(&widgets[i])
	}

	if !hasExplicitShow {
		ApplyCrossWidgetConstraints(widgets, formationMode)
	}

	// Calculate layout zones
	tokens := DefaultDesignTokens()
	for i := range widgets {
		widgets[i].Zones = CalculateZones(widgets[i].Atoms, tokens)
	}

	for i := range widgets {
		for ai := range widgets[i].Atoms {
			if color, ok := params.ColorMap[widgets[i].Atoms[ai].FieldName]; ok {
				if widgets[i].Atoms[ai].Meta == nil {
					widgets[i].Atoms[ai].Meta = make(map[string]interface{})
				}
				widgets[i].Atoms[ai].Meta["color"] = color
			}
		}
		if params.Direction != "" {
			if widgets[i].Meta == nil {
				widgets[i].Meta = make(map[string]interface{})
			}
			widgets[i].Meta["direction"] = params.Direction
		}
	}

	return widgets, formationMode
}

// =============================================================================
// INVARIANT CHECKS -- must hold for ALL parameter combinations
// =============================================================================

type violations struct {
	count int
	first string
}

func (v *violations) add(msg string) {
	v.count++
	if v.first == "" {
		v.first = msg
	}
}

func checkInvariants(widgets []domain.Widget, mode domain.FormationType, params fuzzParams, v *violations) {
	for wi, w := range widgets {
		// I1: No nil atom values
		for ai, a := range w.Atoms {
			if a.Value == nil {
				v.add(fmt.Sprintf("I1 nil value: [%s] w[%d].a[%d](%s)", params, wi, ai, a.FieldName))
			}
		}

		// I2: All display values are known
		for ai, a := range w.Atoms {
			d := string(a.Display)
			if d != "" && !AllValidDisplays[d] {
				v.add(fmt.Sprintf("I2 bad display: [%s] w[%d].a[%d](%s) display=%q", params, wi, ai, a.FieldName, d))
			}
		}

		// I3: All format values are known
		validFormats := map[domain.AtomFormat]bool{
			"": true, domain.FormatCurrency: true, domain.FormatStars: true,
			domain.FormatStarsText: true, domain.FormatStarsCompact: true,
			domain.FormatPercent: true, domain.FormatNumber: true,
			domain.FormatDate: true, domain.FormatText: true,
		}
		for ai, a := range w.Atoms {
			if !validFormats[a.Format] {
				v.add(fmt.Sprintf("I3 bad format: [%s] w[%d].a[%d](%s) format=%q", params, wi, ai, a.FieldName, a.Format))
			}
		}

		// I4: EntityRef present
		if w.EntityRef == nil || w.EntityRef.ID == "" {
			v.add(fmt.Sprintf("I4 no entityRef: [%s] w[%d]", params, wi))
		}

		// I5: No duplicate field names
		seen := make(map[string]int)
		for _, a := range w.Atoms {
			if a.FieldName != "" {
				seen[a.FieldName]++
				if seen[a.FieldName] > 1 {
					v.add(fmt.Sprintf("I5 dup field: [%s] w[%d] field=%q count=%d", params, wi, a.FieldName, seen[a.FieldName]))
				}
			}
		}

		// I6: Max 2 badges (W1)
		badgeCount := 0
		for _, a := range w.Atoms {
			if strings.HasPrefix(string(a.Display), "badge") {
				badgeCount++
			}
		}
		if badgeCount > 2 {
			v.add(fmt.Sprintf("I6 >2 badges: [%s] w[%d] badges=%d", params, wi, badgeCount))
		}

		// I7: Max 5 tags (W2)
		tagCount := 0
		for _, a := range w.Atoms {
			if strings.HasPrefix(string(a.Display), "tag") {
				tagCount++
			}
		}
		if tagCount > 5 {
			v.add(fmt.Sprintf("I7 >5 tags: [%s] w[%d] tags=%d", params, wi, tagCount))
		}

		// I8: Max 1 h1/h2 (W4)
		h1h2 := 0
		for _, a := range w.Atoms {
			d := string(a.Display)
			if d == "h1" || d == "h2" {
				h1h2++
			}
		}
		if h1h2 > 1 {
			v.add(fmt.Sprintf("I8 >1 h1/h2: [%s] w[%d] count=%d", params, wi, h1h2))
		}

		// I9: Direction propagated
		if params.Direction != "" {
			if w.Meta == nil || w.Meta["direction"] != params.Direction {
				v.add(fmt.Sprintf("I9 direction: [%s] w[%d] expected=%q got=%v", params, wi, params.Direction, w.Meta))
			}
		}

		// I10: Color overrides applied
		for _, a := range w.Atoms {
			if color, ok := params.ColorMap[a.FieldName]; ok {
				actual, _ := a.Meta["color"].(string)
				if actual != color {
					v.add(fmt.Sprintf("I10 color: [%s] w[%d] field=%s expected=%q got=%q", params, wi, a.FieldName, color, actual))
				}
			}
		}
	}

	// I11: Widget count matches entity count
	if len(widgets) != params.Count {
		v.add(fmt.Sprintf("I11 widget count: [%s] expected=%d got=%d", params, params.Count, len(widgets)))
	}

	// --- Zone invariants ---
	for wi, w := range widgets {
		atomCount := len(w.Atoms)
		if atomCount == 0 {
			continue // no atoms -> no zones expected
		}

		// I12: All indices in zones are valid (0 <= idx < len(atoms))
		for zi, z := range w.Zones {
			for _, idx := range z.AtomIndices {
				if idx < 0 || idx >= atomCount {
					v.add(fmt.Sprintf("I12 bad zone index: [%s] w[%d].z[%d] idx=%d atoms=%d", params, wi, zi, idx, atomCount))
				}
			}
		}

		// I13: Each atom in exactly one zone (no missing, no duplicates)
		atomInZone := make(map[int]int) // atom index -> zone count
		for _, z := range w.Zones {
			for _, idx := range z.AtomIndices {
				atomInZone[idx]++
			}
		}
		for ai := 0; ai < atomCount; ai++ {
			count := atomInZone[ai]
			if count == 0 {
				v.add(fmt.Sprintf("I13 atom not in zone: [%s] w[%d] atom[%d](%s)", params, wi, ai, w.Atoms[ai].FieldName))
			} else if count > 1 {
				v.add(fmt.Sprintf("I13 atom in %d zones: [%s] w[%d] atom[%d](%s)", count, params, wi, ai, w.Atoms[ai].FieldName))
			}
		}

		// I14: All zone types are valid
		validZoneTypes := map[domain.ZoneType]bool{
			domain.ZoneHero: true, domain.ZoneRow: true, domain.ZoneStack: true,
			domain.ZoneFlow: true, domain.ZoneGrid: true, domain.ZoneCollapsed: true,
		}
		for zi, z := range w.Zones {
			if !validZoneTypes[z.Type] {
				v.add(fmt.Sprintf("I14 bad zone type: [%s] w[%d].z[%d] type=%q", params, wi, zi, z.Type))
			}
		}

		// I15: Image atoms only in hero zones
		for zi, z := range w.Zones {
			if z.Type == domain.ZoneHero {
				continue
			}
			for _, idx := range z.AtomIndices {
				if idx < atomCount && w.Atoms[idx].Type == domain.AtomTypeImage {
					v.add(fmt.Sprintf("I15 image in non-hero: [%s] w[%d].z[%d](%s) atom[%d]", params, wi, zi, z.Type, idx))
				}
			}
		}

		// I16: Collapsed zone only appears immediately after a flow zone
		for zi, z := range w.Zones {
			if z.Type == domain.ZoneCollapsed {
				if zi == 0 || w.Zones[zi-1].Type != domain.ZoneFlow {
					v.add(fmt.Sprintf("I16 collapsed without flow: [%s] w[%d].z[%d]", params, wi, zi))
				}
			}
		}

		// I17: No empty zones
		for zi, z := range w.Zones {
			if len(z.AtomIndices) == 0 {
				v.add(fmt.Sprintf("I17 empty zone: [%s] w[%d].z[%d] type=%s", params, wi, zi, z.Type))
			}
		}
	}
}

// =============================================================================
// TEST 1: Exhaustive structural axes
// count(20) x layout(5) x size(5) x direction(3) x show(15) x hide(14) = 315,000
// =============================================================================

func TestExhaustiveStructural(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	// Pre-generate product pools for each count
	productPools := make(map[int][]domain.Product)
	for _, c := range fuzzCounts {
		productPools[c] = generateFuzzProducts(rng, c)
	}

	v := &violations{}
	total := 0

	for _, count := range fuzzCounts {
		products := productPools[count]
		for _, layout := range fuzzLayouts {
			for _, size := range fuzzSizes {
				for _, dir := range fuzzDirections {
					for _, show := range fuzzShowOptions {
						for _, hide := range fuzzHideOptions {
							params := fuzzParams{
								Count:      count,
								Layout:     layout,
								Size:       size,
								Direction:  dir,
								ShowFields: show,
								HideFields: hide,
							}
							widgets, mode := runPipeline(params, products)
							checkInvariants(widgets, mode, params, v)
							total++
						}
					}
				}
			}
		}
	}

	if v.count > 0 {
		t.Errorf("FAIL: %d violations in %d combinations. First: %s", v.count, total, v.first)
	} else {
		t.Logf("PASS: %d structural combinations, all invariants hold", total)
	}
}

// =============================================================================
// TEST 2: Exhaustive per-atom transforms
// display(23) x format(8) x field(13) = 2,392 -- each applied to 6-product grid
// =============================================================================

func TestExhaustiveAtomTransforms(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	products := generateFuzzProducts(rng, 6)

	v := &violations{}
	total := 0

	for _, field := range fuzzFields {
		for _, display := range fuzzDisplays {
			for _, format := range fuzzFormats {
				params := fuzzParams{
					Count:            6,
					ShowFields:       []string{field},
					DisplayOverrides: map[string]string{field: display},
					FormatOverrides:  map[string]string{field: format},
				}
				widgets, mode := runPipeline(params, products)
				checkInvariants(widgets, mode, params, v)
				total++
			}
		}
	}

	if v.count > 0 {
		t.Errorf("FAIL: %d violations in %d combinations. First: %s", v.count, total, v.first)
	} else {
		t.Logf("PASS: %d atom transform combinations, all invariants hold", total)
	}
}

// =============================================================================
// TEST 3: Random fuzz with ALL overrides active (10,000 iterations)
// =============================================================================

func TestRandomFuzzCombined(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	v := &violations{}
	total := 10_000

	for i := 0; i < total; i++ {
		params := randomFuzzParams(rng)
		products := generateFuzzProducts(rng, params.Count)
		widgets, mode := runPipeline(params, products)
		checkInvariants(widgets, mode, params, v)
	}

	if v.count > 0 {
		t.Errorf("FAIL: %d violations in %d random combinations. First: %s", v.count, total, v.first)
	} else {
		t.Logf("PASS: %d random combinations, all invariants hold", total)
	}
}

// randomFuzzParams generates a random valid parameter set with all overrides
func randomFuzzParams(rng *rand.Rand) fuzzParams {
	p := fuzzParams{
		Count:            rng.Intn(20) + 1,
		Layout:           fuzzLayouts[rng.Intn(len(fuzzLayouts))],
		Size:             fuzzSizes[rng.Intn(len(fuzzSizes))],
		Direction:        fuzzDirections[rng.Intn(len(fuzzDirections))],
		DisplayOverrides: make(map[string]string),
		FormatOverrides:  make(map[string]string),
		ColorMap:         make(map[string]string),
	}

	// Random show fields (0..6)
	n := rng.Intn(7)
	if n > 0 {
		perm := rng.Perm(len(fuzzFields))
		for i := 0; i < n && i < len(perm); i++ {
			p.ShowFields = append(p.ShowFields, fuzzFields[perm[i]])
		}
	}

	// Random hide fields (0..3)
	n = rng.Intn(4)
	if n > 0 {
		perm := rng.Perm(len(fuzzFields))
		for i := 0; i < n && i < len(perm); i++ {
			p.HideFields = append(p.HideFields, fuzzFields[perm[i]])
		}
	}

	// Random order (0..5)
	n = rng.Intn(6)
	if n > 0 {
		perm := rng.Perm(len(fuzzFields))
		for i := 0; i < n && i < len(perm); i++ {
			p.OrderFields = append(p.OrderFields, fuzzFields[perm[i]])
		}
	}

	// Random display overrides (0..4)
	for i := 0; i < rng.Intn(5); i++ {
		p.DisplayOverrides[fuzzFields[rng.Intn(len(fuzzFields))]] = fuzzDisplays[rng.Intn(len(fuzzDisplays))]
	}

	// Random format overrides (0..3)
	for i := 0; i < rng.Intn(4); i++ {
		p.FormatOverrides[fuzzFields[rng.Intn(len(fuzzFields))]] = fuzzFormats[rng.Intn(len(fuzzFormats))]
	}

	// Random color overrides (0..3)
	for i := 0; i < rng.Intn(4); i++ {
		p.ColorMap[fuzzFields[rng.Intn(len(fuzzFields))]] = fuzzColors[rng.Intn(len(fuzzColors))]
	}

	return p
}
