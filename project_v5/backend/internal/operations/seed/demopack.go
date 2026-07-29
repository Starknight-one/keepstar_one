package seed

// Demo seed packs (V2_SPEC.md L6 + ruling R3): realistic starter data
// assembled PER BUSINESS CLASS and seeded at assembly time by the manifest
// applier's seed_demo_data step. Without it both surfaces render empty and
// the L4 demonstration ("book a viewing → the lead appears in the CRM") has
// nothing to demonstrate on.
//
// A pack ships two planes, each through the door real data uses:
//   - Listings → the admin catalog import (same multipart route the uploader
//     posts to), rendered here as a CSV table with a stable SKU per row, so
//     a re-import converges instead of duplicating (R22).
//   - Leads / Contacts → the entity plane, mapped onto whatever entity the
//     conversation actually defined for this tenant (the model names the
//     fields; the pack adapts, never the other way round).
//
// Everything a pack writes carries the demo flag (DemoFlagKey) — L6's
// "purged when real data arrives". The purge itself is not built yet; the
// flag is what makes it possible. When it IS built it must delete the
// TENANT's listings (catalog.products rows) and the flagged entity records —
// never catalog.master_products: masters are cross-tenant (admin matches
// incoming rows on LOWER(sku) with no tenant scope and the first seeder owns
// the row), so deleting one would rip the product out from under every other
// tenant that ever matched it. The same global namespace is why every demo
// SKU carries DemoSKUPrefix.

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
)

const (
	// DemoFlagKey marks every seeded row: a catalog tier2 attribute
	// `demo: true` on listings, a data key `demo: true` on entity records.
	DemoFlagKey = "demo"
	// DemoPackKey records WHICH pack wrote an entity record; the re-run
	// exists-check queries on it (data->>'demoPack').
	DemoPackKey = "demoPack"
	// DemoRecordKey is the record's stable identity inside its pack — the
	// idempotency key: a re-run skips keys already present (R22).
	DemoRecordKey = "demoKey"
	// DemoSource is written into the tenant's source-ish field so demo
	// records are distinguishable in the CRM at a glance.
	DemoSource = "demo"
	// DemoSKUPrefix namespaces every seeded catalog SKU. admin's
	// catalog.master_products.sku is a GLOBAL, tenant-agnostic unique key —
	// ingest matches LOWER(mp.sku) = LOWER(incoming.sku) across all tenants
	// and a matched master is reused as-is. A demo row published under a
	// plain unit code ("VM-0803", "LP-0201" — exactly the shape real realty
	// and parts CSVs use) would therefore capture the next tenant that
	// uploads that SKU for real: their listing would render our photo, our
	// description and our tier2 attributes. The prefix keeps demo masters in
	// a namespace nobody types by accident.
	DemoSKUPrefix = "KS-DEMO-"
)

// DemoPack is one business class's seed pack.
type DemoPack struct {
	// Name identifies the pack ("realty") — recorded on every entity record
	// and in the step result.
	Name string
	// FileName is the import file name. STABLE: admin derives each row's
	// source id from it, so re-imports converge on the same rows.
	FileName string
	// Listings are catalog rows. A row is either a standalone object or one
	// unit of a two-level group (V2_SPEC §4) — see DemoListing.ComplexRef.
	Listings []DemoListing
	// LeadStatuses is the pack's own pipeline vocabulary. The tenant's real
	// value set (whatever the conversation defined) wins at seed time; this
	// list is what the pack's statuses are drawn from and what they are
	// mapped FROM positionally when the vocabularies differ.
	LeadStatuses []domain.ValueSetEntry
	Leads        []DemoLead
	Contacts     []DemoContact
}

// DemoListing is one catalog row. Money is USD dollars (the import parses
// dollars into cents, R18). ComplexRef empty = standalone object; rows
// sharing a ComplexRef are the units of one group card.
type DemoListing struct {
	SKU          string
	Name         string
	Description  string
	PriceUSD     int
	ImageURL     string
	ComplexRef   string // "" = standalone
	ComplexName  string
	Unit         string // unit label inside the group, e.g. "Unit 1104"
	DealType     string // sale | rent (rent prices are monthly)
	PropertyType string
	Bedrooms     int
	Bathrooms    int
	AreaM2       int
	Floor        int
	District     string
	City         string
	YearBuilt    int
	Parking      int
	Furnished    bool
}

// DemoLead is one CRM lead. Phone numbers are E.164 — the capture path
// validates them for real, so demo data must survive the same check.
// DaysAhead/Hour place the preferred visit time RELATIVE to seed time
// (negative = already happened), so a pack seeded any month still reads
// like live data.
type DemoLead struct {
	Key       string
	Name      string
	Phone     string
	Email     string
	Message   string
	Status    string
	DaysAhead int
	Hour      int
}

// DemoContact is one CRM contact (V2_SPEC §4: the CRM's Contacts section).
type DemoContact struct {
	Key   string
	Name  string
	Phone string
	Email string
	Role  string
	Note  string
}

// PackForVertical picks the pack for a business class (L6: "realty agency →
// realty pack"). The vertical is the tenant's own free-form words, so the
// match is on meaning-bearing substrings. Unknown business class → no pack:
// seeding a florist with apartments is worse than seeding nothing.
//
// The vertical is whatever the visitor said, in whatever language the
// conversation ran in — an English-only hint list leaves a Brazilian or
// Russian realtor with two empty tabs, so the words they actually use are
// listed too.
func PackForVertical(vertical string) (DemoPack, bool) {
	v := strings.ToLower(vertical)
	for _, hint := range []string{
		"real estate", "real-estate", "realty", "realtor", "property",
		"properties", "housing", "brokerage", "apartment", "letting",
		"estate agen", // UK: estate agency / estate agent
		// pt / es
		"imobili", "inmobili", "imóve", "imove", "corretor",
		// ru
		"недвиж", "риэлт", "риелт",
	} {
		if strings.Contains(v, hint) {
			return RealtyDemoPack(), true
		}
	}
	return DemoPack{}, false
}

// listingCSVHeaders is the import table's header row. sku / name /
// description / price / image_url are the columns admin's mapper claims;
// everything else passes through as typed tier2 attributes (camelCase
// keys — building_group → buildingGroup), which is how the grouping, the
// facets and the demo flag reach the storefront.
//
// The group key is deliberately NOT called anything id-shaped: when admin
// runs its LLM column mapper, an "…_ref"/"…_id" column carrying slug-like
// values ("vista-marina") is a candidate for the sku slot, and ApplyMapping
// lets the later column win — all three units of one complex would collapse
// onto a single master with processed still reporting the full row count.
var listingCSVHeaders = []string{
	"sku", "name", "description", "price", "image_url",
	"complex", "building_group", "unit", "deal_type", "property_type",
	"bedrooms", "bathrooms", "area_m2", "floor", "district", "city",
	"year_built", "parking", "furnished", DemoFlagKey,
}

// ListingsCSV renders the pack's listings as the import table. Deterministic:
// the same pack always produces byte-identical output, so a re-import is a
// no-op convergence rather than a second catalog.
func (p DemoPack) ListingsCSV() ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(listingCSVHeaders); err != nil {
		return nil, err
	}
	for _, l := range p.Listings {
		row := []string{
			l.SKU, l.Name, l.Description, strconv.Itoa(l.PriceUSD), l.ImageURL,
			l.ComplexName, l.ComplexRef, l.Unit, l.DealType, l.PropertyType,
			strconv.Itoa(l.Bedrooms), strconv.Itoa(l.Bathrooms), strconv.Itoa(l.AreaM2),
			strconv.Itoa(l.Floor), l.District, l.City, strconv.Itoa(l.YearBuilt),
			strconv.Itoa(l.Parking), strconv.FormatBool(l.Furnished), "true",
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Groups returns the pack's two-level groups: ComplexRef → its unit rows,
// in pack order. Standalone rows are not groups and never appear here.
func (p DemoPack) Groups() map[string][]DemoListing {
	out := map[string][]DemoListing{}
	for _, l := range p.Listings {
		if l.ComplexRef == "" {
			continue
		}
		out[l.ComplexRef] = append(out[l.ComplexRef], l)
	}
	return out
}

// ─── mapping the pack onto the tenant's own entity shapes ────────────────
//
// The conversation names the fields (leadName vs name vs clientName), so the
// pack resolves its values onto the tenant's definition by ROLE — type first
// (phone/email/datetime are typed), key hints second. A role the tenant does
// not model is simply not written.

// fieldRole is the semantic slot a pack value wants.
type fieldRole int

const (
	roleName fieldRole = iota
	rolePhone
	roleEmail
	roleNote
	roleSource
	roleWhen
	roleKind
)

// roleHints are the key/label substrings each role recognises, in priority
// order.
var roleHints = map[fieldRole][]string{
	roleName:   {"fullname", "contactname", "clientname", "customername", "name"},
	rolePhone:  {"phone", "mobile", "whatsapp", "tel"},
	roleEmail:  {"email", "mail"},
	roleNote:   {"message", "note", "comment", "request", "details", "description"},
	roleSource: {"source", "channel", "origin"},
	roleWhen:   {"preferredtime", "time", "date", "when", "schedule"},
	roleKind:   {"role", "relationship", "kind", "segment", "type"},
}

// roleTypes are the field types that answer a role outright (typed fields
// beat name guessing).
var roleTypes = map[fieldRole][]domain.FieldType{
	rolePhone: {domain.FieldPhone},
	roleEmail: {domain.FieldEmail},
	roleWhen:  {domain.FieldDatetime},
}

// resolveField returns the definition's field key for a role, "" when the
// tenant does not model it.
func resolveField(def *domain.EntityDefinition, role fieldRole) string {
	if def == nil {
		return ""
	}
	for _, want := range roleTypes[role] {
		for _, f := range def.Fields {
			if f.Type == want {
				return f.Key
			}
		}
	}
	for _, hint := range roleHints[role] {
		for _, f := range def.Fields {
			if !roleFieldTypeOK(role, f.Type) {
				continue
			}
			if strings.Contains(strings.ToLower(f.Key), hint) ||
				strings.Contains(strings.ToLower(f.Label), hint) {
				return f.Key
			}
		}
	}
	if role == roleName && def.Display.TitleField != "" {
		return def.Display.TitleField
	}
	return ""
}

// fieldTypeOf returns a definition field's type ("" when absent).
func fieldTypeOf(def *domain.EntityDefinition, key string) domain.FieldType {
	for _, f := range def.Fields {
		if f.Key == key {
			return f.Type
		}
	}
	return ""
}

// roleFieldTypeOK keeps a hint match from writing text into a number or a
// status field.
func roleFieldTypeOK(role fieldRole, t domain.FieldType) bool {
	switch role {
	case roleWhen:
		return t == domain.FieldDatetime || t == domain.FieldDate
	case rolePhone:
		return t == domain.FieldPhone || t == domain.FieldText
	case roleEmail:
		return t == domain.FieldEmail || t == domain.FieldText
	default:
		return t == domain.FieldText
	}
}

// StatusFieldOf returns the definition's status field key and the value-set
// slug it validates against ("" / "" when the entity has no status). The
// applier's define_entity step infers status_field when the model omitted
// it; the enum fallback here covers definitions written any other way.
func StatusFieldOf(def *domain.EntityDefinition) (field, valueSetRef string) {
	if def == nil {
		return "", ""
	}
	field = def.StatusField
	if field == "" {
		for _, f := range def.Fields {
			if f.Type == domain.FieldEnum &&
				(strings.Contains(strings.ToLower(f.Key), "status") ||
					strings.Contains(strings.ToLower(f.Key), "stage")) {
				field = f.Key
				break
			}
		}
	}
	if field == "" {
		return "", ""
	}
	for _, f := range def.Fields {
		if f.Key == field {
			return field, f.ValueSetRef
		}
	}
	return field, ""
}

// LeadRecordData maps one pack lead onto the tenant's lead-ish entity.
// vs is the tenant's value set for the status field (nil when the tenant
// has none — the pack's own status is then written verbatim). now anchors
// the relative preferred time.
func (p DemoPack) LeadRecordData(def *domain.EntityDefinition, vs *domain.ValueSet, lead DemoLead, now time.Time) map[string]any {
	data := map[string]any{
		DemoFlagKey:   true,
		DemoPackKey:   p.Name,
		DemoRecordKey: lead.Key,
	}
	put(data, resolveField(def, roleName), lead.Name)
	put(data, resolveField(def, rolePhone), lead.Phone)
	put(data, resolveField(def, roleEmail), lead.Email)
	put(data, resolveField(def, roleNote), lead.Message)
	put(data, resolveField(def, roleSource), DemoSource)
	if key := resolveField(def, roleWhen); key != "" {
		day := now.AddDate(0, 0, lead.DaysAhead)
		when := time.Date(day.Year(), day.Month(), day.Day(), lead.Hour, 0, 0, 0, day.Location())
		// A date-typed field takes a date; only datetime takes the clock.
		if fieldTypeOf(def, key) == domain.FieldDate {
			data[key] = when.Format("2006-01-02")
		} else {
			data[key] = when.Format(time.RFC3339)
		}
	}
	if field, _ := StatusFieldOf(def); field != "" {
		data[field] = p.mapStatus(vs, lead.Status)
	}
	return data
}

// ContactRecordData maps one pack contact onto the tenant's contact-ish
// entity. Contacts carry no pipeline status — a status-bearing contact
// entity keeps whatever default its definition declares.
func (p DemoPack) ContactRecordData(def *domain.EntityDefinition, contact DemoContact) map[string]any {
	data := map[string]any{
		DemoFlagKey:   true,
		DemoPackKey:   p.Name,
		DemoRecordKey: contact.Key,
	}
	put(data, resolveField(def, roleName), contact.Name)
	put(data, resolveField(def, rolePhone), contact.Phone)
	put(data, resolveField(def, roleEmail), contact.Email)
	put(data, resolveField(def, roleNote), contact.Note)
	put(data, resolveField(def, roleKind), contact.Role)
	put(data, resolveField(def, roleSource), DemoSource)
	return data
}

// mapStatus translates a pack status into the tenant's vocabulary: exact
// value, then label, then the stage's WORD CLASS, then the pipeline
// position. No value set → the pack's own status.
//
// Everything after the exact/label match runs against the tenant's
// non-losing stages only: no pack lead is lost (the closed one reads
// "Signed for the Urca colonial three-bedroom"), so a pipeline that ends in
// {…, won, lost} must not swallow it into "Lost" — which is exactly what
// raw position does on the very common {new, working, won, lost} set.
func (p DemoPack) mapStatus(vs *domain.ValueSet, want string) string {
	if vs == nil || len(vs.Values) == 0 {
		return want
	}
	for _, v := range vs.Values {
		if strings.EqualFold(v.Value, want) {
			return v.Value
		}
	}
	label := want
	for _, e := range p.LeadStatuses {
		if e.Value == want {
			label = e.Label
			break
		}
	}
	for _, v := range vs.Values {
		if strings.EqualFold(v.Label, label) || strings.EqualFold(v.Value, label) {
			return v.Value
		}
	}

	open := openStages(vs.Values)
	for _, syn := range statusSynonyms[want] {
		for _, v := range open {
			if strings.Contains(strings.ToLower(v.Value), syn) ||
				strings.Contains(strings.ToLower(v.Label), syn) {
				return v.Value
			}
		}
	}
	// The pack's terminal stage belongs at the END of the tenant's pipeline,
	// not at its own index (a 6-stage pipeline would land it mid-funnel).
	if len(p.LeadStatuses) > 0 && p.LeadStatuses[len(p.LeadStatuses)-1].Value == want {
		return open[len(open)-1].Value
	}
	for i, e := range p.LeadStatuses {
		if e.Value != want {
			continue
		}
		if i >= len(open) {
			i = len(open) - 1
		}
		return open[i].Value
	}
	return open[0].Value
}

// statusSynonyms are the words businesses actually use for each stage of the
// packs' pipeline — checked as substrings against the tenant's own value set
// before falling back to position.
var statusSynonyms = map[string][]string{
	"new":            {"new", "novo", "fresh", "incoming", "inbox", "open"},
	"contacted":      {"contact", "contato", "called", "reached", "working", "qualif", "follow"},
	"showing_booked": {"showing", "viewing", "visit", "book", "schedul", "appointment", "meeting", "tour"},
	"closed":         {"closed", "won", "sold", "signed", "deal", "complete", "success"},
}

// lossWords mark a stage that means the lead did NOT convert — in the same
// languages PackForVertical accepts, because a tenant who described itself in
// Portuguese names its pipeline in Portuguese too.
var lossWords = []string{
	"lost", "lose", "cancel", "reject", "declin", "fail", "dead",
	"abandon", "spam", "junk", "unqualified", "not interested",
	"perdid", "rejeitad", "rechaz", "descartad", "recusad", // pt / es
	"потер", "отказ", "проигр", "неудач", // ru
}

// openStages drops the tenant's losing stages; if a pipeline is nothing but
// losing stages (it never is in practice) the full set is returned so the
// mapping still has somewhere to land.
func openStages(values []domain.ValueSetEntry) []domain.ValueSetEntry {
	out := make([]domain.ValueSetEntry, 0, len(values))
	for _, v := range values {
		text := strings.ToLower(v.Value + " " + v.Label)
		lost := false
		for _, w := range lossWords {
			if strings.Contains(text, w) {
				lost = true
				break
			}
		}
		if !lost {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return values
	}
	return out
}

// PickEntity chooses the definition a pack plane writes into: the first
// definition whose slug or name carries one of the hints, most specific
// hint first. Returns nil when the tenant modelled nothing of the kind.
func PickEntity(defs []domain.EntityDefinition, hints []string) *domain.EntityDefinition {
	for _, hint := range hints {
		for i := range defs {
			d := &defs[i]
			if strings.Contains(strings.ToLower(d.Slug), hint) ||
				strings.Contains(strings.ToLower(d.Name), hint) {
				return d
			}
		}
	}
	return nil
}

// LeadEntityHints / ContactEntityHints are the vocabularies the two planes
// recognise, most specific first. The conversation is free to name its
// entities anything; these are the words businesses actually use.
var (
	LeadEntityHints    = []string{"lead", "inquiry", "enquiry", "request", "booking", "showing", "viewing", "prospect"}
	ContactEntityHints = []string{"contact", "client", "customer", "buyer", "owner"}
)

// put writes a non-empty value under a resolved key; an unresolved key ("")
// or an empty value writes nothing.
func put(data map[string]any, key, value string) {
	if key == "" || value == "" {
		return
	}
	data[key] = value
}

// String renders the pack for logs.
func (p DemoPack) String() string {
	return fmt.Sprintf("%s pack: %d listings (%d groups), %d leads, %d contacts",
		p.Name, len(p.Listings), len(p.Groups()), len(p.Leads), len(p.Contacts))
}
