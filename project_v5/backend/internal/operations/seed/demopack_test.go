package seed

import (
	"encoding/csv"
	"regexp"
	"strings"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

// The realty pack is the realtor reference case's demo data (V2_SPEC.md
// L6/R3 + §4). These are the properties the flow depends on — a pack that
// loses any of them produces a storefront that cannot be shown or a lead
// the capture path would reject.

// E.164: the capture operation validates phones for real (a live run proved
// it by rejecting one), so seeded contact data must pass the same bar.
var e164 = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

func TestRealtyPackShape(t *testing.T) {
	p := RealtyDemoPack()

	groups := p.Groups()
	standalone := 0
	for _, l := range p.Listings {
		if l.ComplexRef == "" {
			standalone++
		}
	}
	// §4: a listing is a GROUP (complex with units) or a standalone object.
	if listings := len(groups) + standalone; listings < 10 || listings > 12 {
		t.Errorf("pack offers %d listings (%d groups + %d standalone), want 10-12",
			listings, len(groups), standalone)
	}
	if len(groups) < 4 {
		t.Errorf("pack has %d two-level groups, want at least 4 — the group card with unit selection is the storefront's shape", len(groups))
	}
	for ref, units := range groups {
		if len(units) < 2 || len(units) > 4 {
			t.Errorf("group %q has %d units, want 2-4", ref, len(units))
		}
		// Units must differ where a buyer chooses: price, beds, floor.
		prices, floors := map[int]bool{}, map[int]bool{}
		for _, u := range units {
			if u.ComplexName == "" || u.Unit == "" {
				t.Errorf("group %q unit %s misses its complex name or unit label", ref, u.SKU)
			}
			if u.PriceUSD <= 0 || u.AreaM2 <= 0 || u.Bedrooms <= 0 {
				t.Errorf("group %q unit %s is not a plausible offer: price %d, area %d, beds %d",
					ref, u.SKU, u.PriceUSD, u.AreaM2, u.Bedrooms)
			}
			prices[u.PriceUSD] = true
			floors[u.Floor] = true
		}
		if len(prices) != len(units) || len(floors) != len(units) {
			t.Errorf("group %q units repeat a price or a floor — unit selection would be meaningless", ref)
		}
	}

	// Stable + unique SKUs are the import's idempotency (R22): a re-import
	// matches masters on SKU, so duplicates would collapse two units into one.
	//
	// And every SKU must be NAMESPACED: admin matches incoming rows on
	// LOWER(sku) across the whole master table with no tenant scope, and a
	// matched master is reused untouched. A demo row published as "VM-0803"
	// would therefore be adopted by the next tenant who uploads that unit
	// code for real — their listing would render our photo, our description
	// and our demo tier2 attributes.
	seen := map[string]bool{}
	images, descriptions := map[string]string{}, map[string]string{}
	for _, l := range p.Listings {
		if l.SKU == "" || seen[l.SKU] {
			t.Errorf("listing %q has an empty or duplicate SKU %q", l.Name, l.SKU)
		}
		seen[l.SKU] = true
		if !strings.HasPrefix(l.SKU, DemoSKUPrefix) {
			t.Errorf("listing SKU %q is not namespaced with %q — it would capture a real tenant's row in admin's global master SKU space",
				l.SKU, DemoSKUPrefix)
		}
		if l.Name == "" || l.Description == "" || l.ImageURL == "" {
			t.Errorf("listing %q is not presentable: name/description/image required", l.SKU)
		}
		if l.District == "" || l.City == "" {
			t.Errorf("listing %q has no district/city — the storefront filters on them", l.SKU)
		}
		if l.DealType != "sale" && l.DealType != "rent" {
			t.Errorf("listing %q has deal type %q, want sale|rent", l.SKU, l.DealType)
		}
		// R3 is "touch what you're getting": the whole pack lands on ONE
		// storefront grid, so a repeated photo or a copy-pasted description
		// reads as filler in the exact screen the pack exists for.
		if other, dup := images[l.ImageURL]; dup {
			t.Errorf("listing %q reuses the photo of %q", l.SKU, other)
		}
		images[l.ImageURL] = l.SKU
		if other, dup := descriptions[l.Description]; dup {
			t.Errorf("listing %q repeats the description of %q", l.SKU, other)
		}
		descriptions[l.Description] = l.SKU
	}
}

func TestRealtyPackLeadsAndContacts(t *testing.T) {
	p := RealtyDemoPack()

	if len(p.Leads) < 6 || len(p.Leads) > 8 {
		t.Errorf("pack has %d leads, want 6-8", len(p.Leads))
	}
	if len(p.Contacts) < 3 || len(p.Contacts) > 4 {
		t.Errorf("pack has %d contacts, want 3-4", len(p.Contacts))
	}

	// Statuses ⊆ the pack's own pipeline: the seeder maps them onto the
	// tenant's value set, and a status outside the pack's list has no
	// position to map FROM.
	inPipeline := map[string]bool{}
	for _, e := range p.LeadStatuses {
		inPipeline[e.Value] = true
	}
	used := map[string]bool{}
	keys := map[string]bool{}
	for _, l := range p.Leads {
		if !inPipeline[l.Status] {
			t.Errorf("lead %s carries status %q, outside the pack's value set", l.Key, l.Status)
		}
		used[l.Status] = true
		if !e164.MatchString(l.Phone) {
			t.Errorf("lead %s phone %q is not E.164 — the capture path would reject it", l.Key, l.Phone)
		}
		if l.Key == "" || keys[l.Key] {
			t.Errorf("lead %q has an empty or duplicate demo key", l.Name)
		}
		keys[l.Key] = true
		if l.Name == "" || l.Message == "" {
			t.Errorf("lead %s reads as filler: name and message are what the CRM row shows", l.Key)
		}
		if l.Hour < 8 || l.Hour > 20 {
			t.Errorf("lead %s wants hour %d — outside any business-hours window", l.Key, l.Hour)
		}
	}
	// A funnel, not a pile: the CRM's first render must show movement.
	if len(used) < 3 {
		t.Errorf("leads only use %d of %d pipeline stages — the seeded CRM shows no funnel", len(used), len(p.LeadStatuses))
	}

	for _, c := range p.Contacts {
		if !e164.MatchString(c.Phone) {
			t.Errorf("contact %s phone %q is not E.164", c.Key, c.Phone)
		}
		if c.Key == "" || keys[c.Key] {
			t.Errorf("contact %q has an empty or duplicate demo key", c.Name)
		}
		keys[c.Key] = true
		if c.Name == "" || c.Note == "" || c.Role == "" {
			t.Errorf("contact %s is not a usable CRM row (name/role/note)", c.Key)
		}
	}
}

// The CSV IS the import contract: admin maps sku/name/description/price/
// image_url and passes every other column through as a typed attribute.
// The demo flag rides in that passthrough (L6's purge hook).
func TestListingsCSVIsTheImportTable(t *testing.T) {
	p := RealtyDemoPack()
	raw, err := p.ListingsCSV()
	if err != nil {
		t.Fatalf("render CSV: %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
	if err != nil {
		t.Fatalf("pack CSV does not parse: %v", err)
	}
	if len(rows) != len(p.Listings)+1 {
		t.Fatalf("CSV has %d rows, want %d listings + header", len(rows), len(p.Listings))
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[h] = i
	}
	for _, want := range []string{"sku", "name", "price", "image_url", "building_group", DemoFlagKey} {
		if _, ok := col[want]; !ok {
			t.Fatalf("CSV header misses %q — the import would drop it", want)
		}
	}
	// Admin maps columns with an LLM when a key is configured, and a second
	// id-shaped column carrying slug-like values is a candidate for the sku
	// slot (the later column wins in ApplyMapping) — three units of one
	// complex would then collapse onto a single master while processed still
	// reports every row. The pack owns its headers: none of them may read as
	// an identifier except sku itself.
	for h := range col {
		if h == "sku" {
			continue
		}
		if strings.HasSuffix(h, "_ref") || strings.HasSuffix(h, "_id") || h == "id" || h == "code" {
			t.Errorf("CSV header %q reads as an identifier — an LLM column mapper may map it onto sku", h)
		}
	}
	for _, row := range rows[1:] {
		if row[col[DemoFlagKey]] != "true" {
			t.Errorf("row %s is not flagged as demo data — L6 could never purge it", row[col["sku"]])
		}
		if row[col["price"]] == "0" {
			t.Errorf("row %s has price 0", row[col["sku"]])
		}
	}

	// Deterministic: a retry must post the same bytes, so admin's SKU match
	// converges instead of building a second catalog.
	again, err := p.ListingsCSV()
	if err != nil {
		t.Fatalf("render CSV twice: %v", err)
	}
	if string(again) != string(raw) {
		t.Error("ListingsCSV is not deterministic — a retry would not converge")
	}
}

func TestPackForVertical(t *testing.T) {
	for _, vertical := range []string{"real estate agency", "Realty brokerage", "property management", "REALTOR"} {
		if _, ok := PackForVertical(vertical); !ok {
			t.Errorf("vertical %q should get the realty pack", vertical)
		}
	}
	// The vertical is the visitor's own words in the conversation's own
	// language. An English-only hint list leaves a Brazilian or Russian
	// realtor with two empty tabs and nothing for L4 to demonstrate on —
	// and the only trace would be a warn log.
	for _, vertical := range []string{
		"agência imobiliária", "corretor de imóveis", "inmobiliaria en Madrid",
		"агентство недвижимости", "риэлторское агентство",
	} {
		if _, ok := PackForVertical(vertical); !ok {
			t.Errorf("vertical %q should get the realty pack", vertical)
		}
	}
	// L6 ships packs PER business class: no pack is better than the wrong one.
	for _, vertical := range []string{"flower shop", "цветочный магазин", "padaria"} {
		if _, ok := PackForVertical(vertical); ok {
			t.Errorf("%q must not be seeded with the realty pack", vertical)
		}
	}
}

// The pack's closing stage means the deal was WON ("Signed for the Urca
// colonial three-bedroom. Keep in touch for referrals."). Mapping it by raw
// position drops that lead into whatever sits at the same index of the
// tenant's pipeline — "Lost" on the very common {new, working, won, lost}
// set, in the one CRM screen the owner demos.
func TestClosedLeadNeverMapsOntoALosingStage(t *testing.T) {
	p := RealtyDemoPack()
	closed := p.Leads[len(p.Leads)-1]
	if closed.Status != "closed" {
		t.Fatalf("fixture drift: last lead carries status %q, expected the pack's terminal stage", closed.Status)
	}
	def := &domain.EntityDefinition{
		Slug:        "lead",
		Fields:      []domain.FieldDef{{Key: "stage", Label: "Stage", Type: domain.FieldEnum, ValueSetRef: "vs"}},
		StatusField: "stage",
	}

	cases := []struct {
		name   string
		values []domain.ValueSetEntry
		want   string
	}{
		{
			name: "four stages ending in won/lost",
			values: []domain.ValueSetEntry{
				{Value: "new", Label: "New"}, {Value: "working", Label: "Working"},
				{Value: "won", Label: "Won"}, {Value: "lost", Label: "Lost"},
			},
			want: "won",
		},
		{
			name: "six stages: the terminal stage is not at the pack's index",
			values: []domain.ValueSetEntry{
				{Value: "new", Label: "New"}, {Value: "contacted", Label: "Contacted"},
				{Value: "viewing", Label: "Viewing"}, {Value: "offer", Label: "Offer"},
				{Value: "won", Label: "Won"}, {Value: "lost", Label: "Lost"},
			},
			want: "won",
		},
		{
			name: "portuguese pipeline, no shared words",
			values: []domain.ValueSetEntry{
				{Value: "novo", Label: "Novo"}, {Value: "em_contato", Label: "Em contato"},
				{Value: "visita", Label: "Visita"}, {Value: "fechado", Label: "Fechado"},
				{Value: "perdido", Label: "Perdido"},
			},
			want: "fechado",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vs := &domain.ValueSet{Slug: "vs", Values: tc.values}
			got, _ := p.LeadRecordData(def, vs, closed, time.Now())["stage"].(string)
			if got != tc.want {
				t.Errorf("closed lead mapped onto %q, want %q", got, tc.want)
			}
		})
	}

	// The middle of the funnel still maps by class, not by luck.
	vs := &domain.ValueSet{Slug: "vs", Values: []domain.ValueSetEntry{
		{Value: "new", Label: "New"}, {Value: "contacted", Label: "Contacted"},
		{Value: "viewing", Label: "Viewing"}, {Value: "offer", Label: "Offer"},
		{Value: "won", Label: "Won"}, {Value: "lost", Label: "Lost"},
	}}
	booked := DemoLead{Key: "x", Status: "showing_booked"}
	if got, _ := p.LeadRecordData(def, vs, booked, time.Now())["stage"].(string); got != "viewing" {
		t.Errorf("a booked showing mapped onto %q, want the tenant's viewing stage", got)
	}
}

// The conversation names the fields; the pack adapts. This is the contract
// that keeps demo data alive when the model calls its entity "client
// request" with a field called "contactPhone".
func TestLeadRecordDataMapsOntoTheTenantsOwnFields(t *testing.T) {
	def := &domain.EntityDefinition{
		Slug: "client_request",
		Name: "Client request",
		Fields: []domain.FieldDef{
			{Key: "clientName", Label: "Client", Type: domain.FieldText},
			{Key: "contactPhone", Label: "Phone", Type: domain.FieldPhone},
			{Key: "contactEmail", Label: "Email", Type: domain.FieldEmail},
			{Key: "notes", Label: "Notes", Type: domain.FieldText},
			{Key: "howTheyFoundUs", Label: "Source", Type: domain.FieldText},
			{Key: "visitAt", Label: "Visit", Type: domain.FieldDatetime},
			{Key: "stage", Label: "Stage", Type: domain.FieldEnum, ValueSetRef: "request_stages"},
		},
		StatusField: "stage",
	}
	// The tenant's OWN vocabulary — different words, same pipeline shape.
	vs := &domain.ValueSet{Slug: "request_stages", Values: []domain.ValueSetEntry{
		{Value: "fresh", Label: "Fresh"},
		{Value: "called", Label: "Called"},
		{Value: "visit_set", Label: "Visit set"},
		{Value: "won", Label: "Won"},
	}}

	p := RealtyDemoPack()
	now := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	lead := p.Leads[0]
	data := p.LeadRecordData(def, vs, lead, now)

	if data["clientName"] != lead.Name {
		t.Errorf("name landed on %v, want field clientName = %q", data, lead.Name)
	}
	if data["contactPhone"] != lead.Phone {
		t.Errorf("phone not mapped onto the typed phone field: %v", data)
	}
	if data["contactEmail"] != lead.Email {
		t.Errorf("email not mapped onto the typed email field: %v", data)
	}
	if data["notes"] != lead.Message {
		t.Errorf("message not mapped onto the note field: %v", data)
	}
	if data["howTheyFoundUs"] != DemoSource {
		t.Errorf("source field = %v, want %q", data["howTheyFoundUs"], DemoSource)
	}
	if _, ok := data["visitAt"].(string); !ok {
		t.Errorf("preferred time not written onto the datetime field: %v", data)
	}
	// Positional mapping: pack "new" is stage 1 → the tenant's stage 1.
	if data["stage"] != "fresh" {
		t.Errorf("status = %v, want the tenant's first stage %q", data["stage"], "fresh")
	}
	if data[DemoFlagKey] != true || data[DemoPackKey] != p.Name || data[DemoRecordKey] != lead.Key {
		t.Errorf("record is not flagged/keyed as demo data: %v", data)
	}

	// A tenant whose statuses happen to match the pack keeps them verbatim.
	own := &domain.ValueSet{Slug: "lead_pipeline", Values: p.LeadStatuses}
	closed := p.Leads[len(p.Leads)-1]
	got := p.LeadRecordData(def, own, closed, now)
	if got["stage"] != closed.Status {
		t.Errorf("matching vocabulary should be kept verbatim: %v != %q", got["stage"], closed.Status)
	}

	// No value set at all → the pack's own status, never an empty cell.
	got = p.LeadRecordData(def, nil, closed, now)
	if got["stage"] != closed.Status {
		t.Errorf("without a value set the pack's own status must be written, got %v", got["stage"])
	}
}

// A tenant that models fewer fields simply gets fewer keys — never a write
// into a field it never defined.
func TestRecordDataWritesOnlyDefinedFields(t *testing.T) {
	def := &domain.EntityDefinition{
		Slug:   "lead",
		Fields: []domain.FieldDef{{Key: "name", Label: "Name", Type: domain.FieldText}},
	}
	p := RealtyDemoPack()
	data := p.LeadRecordData(def, nil, p.Leads[0], time.Now())
	for key := range data {
		switch key {
		case "name", DemoFlagKey, DemoPackKey, DemoRecordKey:
		default:
			t.Errorf("wrote undefined field %q onto a one-field entity", key)
		}
	}
}

func TestPickEntity(t *testing.T) {
	defs := []domain.EntityDefinition{
		{Slug: "property_showing", Name: "Property showing"},
		{Slug: "client", Name: "Client"},
	}
	if got := PickEntity(defs, LeadEntityHints); got == nil || got.Slug != "property_showing" {
		t.Errorf("lead hints picked %v, want property_showing", got)
	}
	if got := PickEntity(defs, ContactEntityHints); got == nil || got.Slug != "client" {
		t.Errorf("contact hints picked %v, want client", got)
	}
	if got := PickEntity([]domain.EntityDefinition{{Slug: "invoice"}}, LeadEntityHints); got != nil {
		t.Errorf("nothing lead-like should match, got %v", got)
	}
}

// A date-typed "when" field takes a date, a datetime field takes the clock:
// the pack adapts to the type the conversation chose.
func TestLeadWhenFieldRespectsTheFieldType(t *testing.T) {
	p := RealtyDemoPack()
	now := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	lead := p.Leads[0]

	dateOnly := &domain.EntityDefinition{Slug: "lead", Fields: []domain.FieldDef{
		{Key: "visitDate", Label: "Visit date", Type: domain.FieldDate},
	}}
	if got, _ := p.LeadRecordData(dateOnly, nil, lead, now)["visitDate"].(string); len(got) != len("2006-01-02") {
		t.Errorf("date field got %q, want a plain date", got)
	}
	withClock := &domain.EntityDefinition{Slug: "lead", Fields: []domain.FieldDef{
		{Key: "visitAt", Label: "Visit at", Type: domain.FieldDatetime},
	}}
	got, _ := p.LeadRecordData(withClock, nil, lead, now)["visitAt"].(string)
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("datetime field got %q: %v", got, err)
	}
	if parsed.Hour() != lead.Hour {
		t.Errorf("datetime field lost the hour: %q, want hour %d", got, lead.Hour)
	}
}
