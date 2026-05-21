// Package usecases — discoveryDraft accumulates a MappingArtifactV3 in
// small, individually-validated steps. Replaces the previous "agent emits
// the whole artifact as one big JSON blob and we validate post-hoc" flow.
//
// Each mutator validates its own args at call time using cached inbox
// state (list of top-level fields + sample values per field). When an
// agent passes a typoed field name, the mutator returns an error
// carrying the three closest candidates by Levenshtein distance, and
// the agent self-corrects in the same run without re-rolling the whole
// artifact.
package usecases

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"keepstar-admin/internal/domain"
)

// discoveryDraft is the in-memory artifact being assembled across one
// discovery run. Mirrors MappingArtifactV3 plus tracking state. Owned by
// runLoop; mutated by tool dispatchers.
type discoveryDraft struct {
	ClassifyingField string
	Branches         []domain.VerticalBranch
	ClassifyRules    []domain.ClassifyRule
	Notes            string

	// toolErrorCount tracks consecutive failures per tool name so the
	// dispatcher can nudge the agent at 3 errors and abort at 5.
	toolErrorCount map[string]int
}

func newDiscoveryDraft() *discoveryDraft {
	return &discoveryDraft{toolErrorCount: map[string]int{}}
}

// Finalize freezes the draft into the canonical MappingArtifactV3. Called
// from the commit tool. BuiltAt is left zero — runLoop stamps it.
func (d *discoveryDraft) Finalize() *domain.MappingArtifactV3 {
	return &domain.MappingArtifactV3{
		Version:          3,
		ClassifyingField: d.ClassifyingField,
		Branches:         append([]domain.VerticalBranch(nil), d.Branches...),
		ClassifyRules:    append([]domain.ClassifyRule(nil), d.ClassifyRules...),
		Notes:            d.Notes,
	}
}

// --- Allowlists. Hard-coded mirrors of the canonical master / cosmetics
// schemas. Keep in sync with catalog_migrations.go and the upsert SQL in
// catalog_v2_writer_adapter.go.

// allowedVerticals is the closed enum apply_v2 understands.
var allowedVerticals = map[string]bool{
	"cosmetics": true, "electronics": true, "furniture": true, "haircare": true,
	"apparel": true, "footwear": true, "food": true, "ski": true, "unknown": true,
}

// masterColumns is the Tier-1 column allowlist for `master.<col>` targets.
var masterColumns = map[string]bool{
	"name": true, "brand": true, "description": true, "sku": true,
	"vertical": true, "image_url": true, "gtin": true,
}

// cosmeticsArrayColumns are text[] columns on master_cosmetics — only valid
// `split` transform targets, never `numeric`/`int`.
var cosmeticsArrayColumns = map[string]bool{
	"skin_type": true, "concern": true, "key_ingredients": true,
	"target_area": true, "free_from": true, "benefits": true,
}

// cosmeticsScalarColumns are the scalar columns on master_cosmetics.
var cosmeticsScalarColumns = map[string]bool{
	"product_form": true, "texture": true, "routine_step": true,
	"routine_time": true, "application_method": true, "scent": true,
	"marketing_claim": true, "how_to_use": true,
	"spf": true, "volume_ml": true, "weight_g": true, "unit_count": true,
}

// cosmeticsNumericColumns is the subset of cosmeticsScalarColumns that
// holds numeric values — these are the only valid targets for
// numeric/int/ml_from_string/g_from_string transforms inside cosmetics.
var cosmeticsNumericColumns = map[string]bool{
	"spf": true, "volume_ml": true, "weight_g": true, "unit_count": true,
}

// allowedClassifySignals is the closed set the DSL parser accepts. Tools
// take this as `signal` enum input, dispatcher composes the canonical
// `<signal> <op> '<value>'` string for persistence — no quote escaping.
var allowedClassifySignals = map[string]bool{
	"product_type": true, "brand": true, "name": true, "tag": true,
}

// allowedClassifyOps mirrors the DSL operator set (matchesRule in
// classify_vertical.go).
var allowedClassifyOps = map[string]bool{
	"contains": true, "=": true,
}

// allowedTransforms is the set of `transform` values applyV2Transform
// understands. Empty string = no transform.
var allowedTransforms = map[string]bool{
	"": true, "lowercase": true, "trim": true, "split": true,
	"ml_from_string": true, "g_from_string": true, "bool_from_yesno": true,
	"int": true, "numeric": true,
}

// fieldNameRe restricts target columns to safe identifiers.
var fieldNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// --- Mutators.

// setClassifyingField records the tenant's category-holding inbox key.
// `inboxFields` is the result of InboxPort.ListFields (cached by caller).
// `sampleStrings` is up to N stringified values seen for the candidate
// field — used to reject obviously-non-categorical fields (all blanks,
// all numbers).
func (d *discoveryDraft) setClassifyingField(field string, inboxFields []string, sampleStrings []string) error {
	field = strings.TrimSpace(field)
	if field == "" {
		return fmt.Errorf("classifying_field: empty")
	}
	if !inSlice(inboxFields, field) {
		return fmt.Errorf("classifying_field %q not in inbox top-level keys; closest: %s",
			field, strings.Join(closestStrings(field, inboxFields, 3), ", "))
	}
	// At least one sample must be a non-blank string. Numeric-only fields
	// (e.g., brand_id) make the classify DSL match nothing.
	hasStringSample := false
	for _, s := range sampleStrings {
		if strings.TrimSpace(s) != "" {
			hasStringSample = true
			break
		}
	}
	if !hasStringSample {
		return fmt.Errorf("classifying_field %q has no non-blank string samples; pick a categorical field", field)
	}
	d.ClassifyingField = field
	return nil
}

// addBranch declares an artifact branch for one vertical. Apply_v2 selects
// a branch per row via ClassifyVertical → FindBranch.
func (d *discoveryDraft) addBranch(vertical string) error {
	vertical = strings.ToLower(strings.TrimSpace(vertical))
	if !allowedVerticals[vertical] {
		return fmt.Errorf("vertical %q not in allowed set: %s", vertical, sortedKeys(allowedVerticals))
	}
	for _, b := range d.Branches {
		if strings.EqualFold(b.Vertical, vertical) {
			return fmt.Errorf("branch %q already declared", vertical)
		}
	}
	d.Branches = append(d.Branches, domain.VerticalBranch{
		Vertical: vertical,
		FieldMap: []domain.FieldMappingRule{},
	})
	return nil
}

// addFieldMapping appends one mapping rule to a branch. This is the most
// frequently called mutator — it carries the most validation.
//
// Parameters:
//   - vertical: which branch the rule belongs to (must already exist).
//   - from: inbox key path (must be present in `inboxFields`).
//   - to: target identifier `<prefix>.<col>` with prefix ∈ {master,cosmetics,tier3}.
//   - transform: optional name from allowedTransforms.
//   - splitDelim: required iff transform=="split"; embedded into the
//     canonical "split:<delim>" form for storage.
//   - deflt: literal default value used when raw[from] is missing.
//   - inboxFields: cached top-level keys; used for typo suggestions.
func (d *discoveryDraft) addFieldMapping(vertical, from, to, transform, splitDelim, deflt string, inboxFields []string) error {
	vertical = strings.ToLower(strings.TrimSpace(vertical))
	branchIdx := -1
	for i, b := range d.Branches {
		if strings.EqualFold(b.Vertical, vertical) {
			branchIdx = i
			break
		}
	}
	if branchIdx < 0 {
		return fmt.Errorf("branch %q not declared; call add_branch first", vertical)
	}

	from = strings.TrimSpace(from)
	if from == "" {
		return fmt.Errorf("from: empty")
	}
	// `from` may carry array-index suffix like `images[0]`. Strip the
	// indices for the inbox-field check; we only confirm the root key
	// exists. apply_v2's getPath handles the indexing.
	fromRoot := stripPathIndices(from)
	if !inSlice(inboxFields, fromRoot) {
		return fmt.Errorf("from=%q not in inbox top-level keys; closest: %s",
			fromRoot, strings.Join(closestStrings(fromRoot, inboxFields, 3), ", "))
	}

	to = strings.TrimSpace(to)
	prefix, col, ok := splitTarget(to)
	if !ok {
		return fmt.Errorf("to=%q malformed; expected <prefix>.<col> with prefix in {master,cosmetics,tier3}", to)
	}
	switch prefix {
	case "master":
		if !masterColumns[col] {
			return fmt.Errorf("to=%q not a master Tier-1 column; allowed: %s", to, sortedKeys(masterColumns))
		}
	case "cosmetics":
		if !cosmeticsArrayColumns[col] && !cosmeticsScalarColumns[col] {
			return fmt.Errorf("to=%q not a master_cosmetics column; arrays: %s; scalars: %s",
				to, sortedKeys(cosmeticsArrayColumns), sortedKeys(cosmeticsScalarColumns))
		}
		if vertical != "cosmetics" {
			return fmt.Errorf("cosmetics.* targets only valid inside cosmetics branch (this branch is %q); route to tier3.<key> instead", vertical)
		}
	case "tier3":
		if !fieldNameRe.MatchString(col) {
			return fmt.Errorf("tier3 key %q malformed; use snake_case [a-z][a-z0-9_]*", col)
		}
	}

	transform = strings.TrimSpace(transform)
	if !allowedTransforms[transform] {
		return fmt.Errorf("transform=%q unknown; allowed: %s", transform, sortedKeys(allowedTransforms))
	}

	// Transform/target compatibility — the class of bugs we want to
	// catch BEFORE apply hits a row and falls into mapping_miss.
	switch transform {
	case "split":
		if splitDelim == "" {
			return fmt.Errorf("transform=split requires split_delim")
		}
		// split returns []string; only cosmetics text[] columns accept that.
		if prefix != "cosmetics" || !cosmeticsArrayColumns[col] {
			return fmt.Errorf("transform=split only valid for cosmetics.<array_col>; %q is not an array target", to)
		}
	case "numeric", "int", "ml_from_string", "g_from_string":
		// Numeric output. Disallow on text master columns and on
		// non-numeric cosmetics scalars.
		if prefix == "master" && col != "gtin" {
			return fmt.Errorf("transform=%q produces a number; master.%s is text — drop the transform or pick a numeric target", transform, col)
		}
		if prefix == "cosmetics" && !cosmeticsNumericColumns[col] {
			return fmt.Errorf("transform=%q produces a number; cosmetics.%s is text or array — pick a numeric cosmetics column (%s)",
				transform, col, sortedKeys(cosmeticsNumericColumns))
		}
		// tier3 accepts anything.
	}

	// Build the canonical FieldMappingRule. For split, fold split_delim
	// into the legacy `split:<delim>` form so apply_v2_transforms
	// recognises it.
	finalTransform := transform
	if transform == "split" {
		finalTransform = "split:" + splitDelim
	}
	rule := domain.FieldMappingRule{
		From:      from,
		To:        to,
		Transform: finalTransform,
		Default:   deflt,
	}

	// Dedup: same (from, to) within branch — error so agent picks one.
	for _, r := range d.Branches[branchIdx].FieldMap {
		if r.From == rule.From && r.To == rule.To {
			return fmt.Errorf("duplicate mapping in branch %q: from=%q to=%q already declared (remove_field_mapping first)", vertical, from, to)
		}
	}

	d.Branches[branchIdx].FieldMap = append(d.Branches[branchIdx].FieldMap, rule)
	return nil
}

// addClassifyRule appends one classify_rules entry. signal/op/value
// arrive as structured args; this method composes the canonical DSL
// string ("<signal> <op> '<value>'") so quote escaping never matters.
// sampleValues, if non-nil, is checked: at least one sample must satisfy
// the rule, otherwise it's a "dead rule" that classifies zero rows.
func (d *discoveryDraft) addClassifyRule(signal, op, value, thenVertical string, sampleValues []string) error {
	signal = strings.ToLower(strings.TrimSpace(signal))
	op = strings.TrimSpace(op)
	value = strings.TrimSpace(value)
	thenVertical = strings.ToLower(strings.TrimSpace(thenVertical))

	if !allowedClassifySignals[signal] {
		return fmt.Errorf("signal=%q not allowed; choose from %s", signal, sortedKeys(allowedClassifySignals))
	}
	if !allowedClassifyOps[op] {
		return fmt.Errorf("op=%q not allowed; choose from %s", op, sortedKeys(allowedClassifyOps))
	}
	if value == "" {
		return fmt.Errorf("value: empty")
	}
	if !allowedVerticals[thenVertical] {
		return fmt.Errorf("then_vertical %q not in allowed set: %s", thenVertical, sortedKeys(allowedVerticals))
	}
	// then_vertical must point to a declared branch — otherwise apply
	// would emit mapping_miss for every matched row.
	hasBranch := false
	for _, b := range d.Branches {
		if strings.EqualFold(b.Vertical, thenVertical) {
			hasBranch = true
			break
		}
	}
	if !hasBranch {
		return fmt.Errorf("then_vertical %q has no matching branch (declare add_branch(%q) first)", thenVertical, thenVertical)
	}

	// Dead-rule check (caller-supplied samples are best-effort).
	if len(sampleValues) > 0 && !ruleMatchesAnySample(op, value, sampleValues) {
		return fmt.Errorf("dead rule: %q %s %q matches 0/%d sampled values; pick a value that actually appears in the data",
			signal, op, value, len(sampleValues))
	}

	whenStr := fmt.Sprintf("%s %s '%s'", signal, op, value)

	// Dedup.
	for _, r := range d.ClassifyRules {
		if strings.EqualFold(r.When, whenStr) && strings.EqualFold(r.ThenVertical, thenVertical) {
			return fmt.Errorf("duplicate rule: %q → %q already declared", whenStr, thenVertical)
		}
	}

	d.ClassifyRules = append(d.ClassifyRules, domain.ClassifyRule{
		When:         whenStr,
		ThenVertical: thenVertical,
	})
	return nil
}

// removeFieldMapping deletes an exact-match rule from one branch.
func (d *discoveryDraft) removeFieldMapping(vertical, from, to string) error {
	vertical = strings.ToLower(strings.TrimSpace(vertical))
	for bi, b := range d.Branches {
		if !strings.EqualFold(b.Vertical, vertical) {
			continue
		}
		for ri, r := range b.FieldMap {
			if r.From == from && r.To == to {
				d.Branches[bi].FieldMap = append(b.FieldMap[:ri], b.FieldMap[ri+1:]...)
				return nil
			}
		}
		return fmt.Errorf("rule not found in branch %q: from=%q to=%q", vertical, from, to)
	}
	return fmt.Errorf("branch %q not declared", vertical)
}

// removeClassifyRule deletes a rule by structured args (recomposes the
// canonical DSL string and matches).
func (d *discoveryDraft) removeClassifyRule(signal, op, value string) error {
	signal = strings.ToLower(strings.TrimSpace(signal))
	op = strings.TrimSpace(op)
	value = strings.TrimSpace(value)
	whenStr := fmt.Sprintf("%s %s '%s'", signal, op, value)
	for i, r := range d.ClassifyRules {
		if strings.EqualFold(r.When, whenStr) {
			d.ClassifyRules = append(d.ClassifyRules[:i], d.ClassifyRules[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("classify_rule not found: %q", whenStr)
}

// --- Helpers.

func inSlice(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// splitTarget breaks "<prefix>.<col>" into its parts. Returns ok=false
// when the prefix isn't one of {master, cosmetics, tier3} or col is
// malformed.
func splitTarget(target string) (prefix, col string, ok bool) {
	idx := strings.IndexByte(target, '.')
	if idx <= 0 || idx == len(target)-1 {
		return "", "", false
	}
	prefix = target[:idx]
	col = target[idx+1:]
	switch prefix {
	case "master", "cosmetics", "tier3":
		// fall through
	default:
		return "", "", false
	}
	if !fieldNameRe.MatchString(col) {
		return "", "", false
	}
	return prefix, col, true
}

// stripPathIndices removes `[N]` suffixes for the inbox-field membership
// check. `images[0].alt` → `images`.
func stripPathIndices(s string) string {
	if idx := strings.IndexAny(s, "[."); idx > 0 {
		return s[:idx]
	}
	return s
}

// ruleMatchesAnySample mirrors classify_vertical.matchesRule semantics
// for the two ops (contains, =) — enough to spot dead rules. Case
// insensitive.
func ruleMatchesAnySample(op, value string, samples []string) bool {
	value = strings.ToLower(value)
	for _, s := range samples {
		sl := strings.ToLower(s)
		switch op {
		case "contains":
			if strings.Contains(sl, value) {
				return true
			}
		case "=":
			if strings.EqualFold(strings.TrimSpace(sl), value) {
				return true
			}
		}
	}
	return false
}

// closestStrings returns up to k candidates from `pool` sorted by
// Levenshtein distance to `target`, ascending. Used for typo suggestions
// in error messages.
func closestStrings(target string, pool []string, k int) []string {
	type ranked struct {
		s string
		d int
	}
	rs := make([]ranked, 0, len(pool))
	for _, p := range pool {
		rs = append(rs, ranked{p, levenshtein(target, p)})
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].d < rs[j].d })
	out := make([]string, 0, k)
	for i := 0; i < len(rs) && i < k; i++ {
		out = append(out, rs[i].s)
	}
	return out
}

// levenshtein returns the edit distance between two strings. Plain
// implementation; pool sizes here are tiny (~50 inbox fields), so
// allocation cost is irrelevant.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = minInt(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func minInt(xs ...int) int {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}
