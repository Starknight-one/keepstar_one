package seed

// The realty demo pack (V2_SPEC.md L6/R3 + §4, the realtor reference case):
// what a Rio agency's workspace looks like the second it is assembled.
//
// 11 listings: 4 two-level GROUPS (a complex with 2–3 units, each unit with
// its own price, beds, floor) + 7 standalone objects — 17 catalog rows in
// total. 8 leads spread across the pipeline and 4 contacts.
//
// Rules the fixture obeys, encoded in demopack_test.go:
//   - phones are E.164 (the capture path validates them for real);
//   - lead statuses come from LeadStatuses, nothing invented;
//   - SKUs are stable, unique AND namespaced with DemoSKUPrefix — admin's
//     master_products.sku is a GLOBAL unique namespace matched case-
//     insensitively across tenants, so a bare unit code like "VM-0803" would
//     be adopted by the next tenant that uploads that SKU for real (their
//     listing would render OUR photo, description and tier2 attributes);
//   - photos and descriptions are unique per row — the storefront shows the
//     whole pack on one grid, and repetition reads as filler;
//   - prices/areas/floors are plausible for the district, because the whole
//     point of R3 is "touch what you're getting", not lorem ipsum.
//
// Photos are public Unsplash images (each URL HEAD-checked 200 when the pack
// was written).

import "keepstar_v5/internal/domain"

// RealtyDemoPack returns the realty agency pack.
func RealtyDemoPack() DemoPack {
	return DemoPack{
		Name:     "realty",
		FileName: "keepstar-demo-realty.csv",
		Listings: realtyListings(),
		// The pipeline the realtor blueprint defines: a lead arrives, gets
		// called, gets a viewing, closes. Colors on the brand ramp — no purple.
		LeadStatuses: []domain.ValueSetEntry{
			{Value: "new", Label: "New", Color: "#5BA4D9"},
			{Value: "contacted", Label: "Contacted", Color: "#F0924A"},
			{Value: "showing_booked", Label: "Showing booked", Color: "#4CAF7D"},
			{Value: "closed", Label: "Closed", Color: "#8A8F98"},
		},
		Leads:    realtyLeads(),
		Contacts: realtyContacts(),
	}
}

// realtyListings: 4 groups (10 units) + 7 standalone objects. Every SKU is
// namespaced (DemoSKUPrefix) — see the file header: admin's master SKU space
// is global, not per tenant.
func realtyListings() []DemoListing {
	return []DemoListing{
		// ── Group 1: Vista Marina Residences, Botafogo (3 units) ──
		{
			SKU: "KS-DEMO-REALTY-VM-0803", ComplexRef: "vista-marina", ComplexName: "Vista Marina Residences", Unit: "Unit 803",
			Name:        "Vista Marina — Unit 803",
			Description: "Two-bedroom on the eighth floor with a wraparound balcony over the bay. Renovated kitchen, building gym and 24h concierge.",
			PriceUSD:    285000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 2, Bathrooms: 2, AreaM2: 86, Floor: 8,
			District: "Botafogo", City: "Rio de Janeiro", YearBuilt: 2016, Parking: 1, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1512917774080-9991f1c4c750?w=1200",
		},
		{
			SKU: "KS-DEMO-REALTY-VM-1104", ComplexRef: "vista-marina", ComplexName: "Vista Marina Residences", Unit: "Unit 1104",
			Name:        "Vista Marina — Unit 1104",
			Description: "Three-bedroom corner unit, morning sun on both facades, suite with walk-in closet and a separate service entrance.",
			PriceUSD:    365000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 3, Bathrooms: 2, AreaM2: 112, Floor: 11,
			District: "Botafogo", City: "Rio de Janeiro", YearBuilt: 2016, Parking: 2, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1502672260266-1c1ef2d93688?w=1200",
		},
		{
			SKU: "KS-DEMO-REALTY-VM-1601", ComplexRef: "vista-marina", ComplexName: "Vista Marina Residences", Unit: "Unit 1601",
			Name:        "Vista Marina — Unit 1601",
			Description: "Top-floor three-bedroom with unobstructed Sugarloaf views, two suites and a 20 m2 terrace off the living room.",
			PriceUSD:    498000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 3, Bathrooms: 3, AreaM2: 140, Floor: 16,
			District: "Botafogo", City: "Rio de Janeiro", YearBuilt: 2016, Parking: 2, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1600585154340-be6161a56a0c?w=1200",
		},

		// ── Group 2: Laranjeiras Park Residences (2 units) ──
		{
			SKU: "KS-DEMO-REALTY-LP-0201", ComplexRef: "laranjeiras-park", ComplexName: "Laranjeiras Park Residences", Unit: "Unit 201",
			Name:        "Laranjeiras Park — Unit 201",
			Description: "Compact one-bedroom facing the inner garden. Built-in wardrobes, induction kitchen, fiber ready.",
			PriceUSD:    164000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 1, Bathrooms: 1, AreaM2: 52, Floor: 2,
			District: "Laranjeiras", City: "Rio de Janeiro", YearBuilt: 2019, Parking: 1, Furnished: true,
			ImageURL: "https://images.unsplash.com/photo-1522708323590-d24dbb6b0267?w=1200",
		},
		{
			SKU: "KS-DEMO-REALTY-LP-0502", ComplexRef: "laranjeiras-park", ComplexName: "Laranjeiras Park Residences", Unit: "Unit 502",
			Name:        "Laranjeiras Park — Unit 502",
			Description: "Two-bedroom with a dedicated home-office nook and blackout glazing toward the street. Pet-friendly condo with a dog wash.",
			PriceUSD:    228000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 2, Bathrooms: 2, AreaM2: 78, Floor: 5,
			District: "Laranjeiras", City: "Rio de Janeiro", YearBuilt: 2019, Parking: 1, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1484154218962-a197022b5858?w=1200",
		},

		// ── Group 3: Leblon Sky Towers (3 units) ──
		{
			SKU: "KS-DEMO-REALTY-LS-1902", ComplexRef: "leblon-sky", ComplexName: "Leblon Sky Towers", Unit: "Unit 1902",
			Name:        "Leblon Sky — Unit 1902",
			Description: "Two-bedroom high-floor unit two blocks from the beach. Concierge, indoor pool and a residents' co-work lounge.",
			PriceUSD:    690000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 2, Bathrooms: 2, AreaM2: 95, Floor: 19,
			District: "Leblon", City: "Rio de Janeiro", YearBuilt: 2021, Parking: 1, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1493809842364-78817add7ffb?w=1200",
		},
		{
			SKU: "KS-DEMO-REALTY-LS-2101", ComplexRef: "leblon-sky", ComplexName: "Leblon Sky Towers", Unit: "Unit 2101",
			Name:        "Leblon Sky — Unit 2101",
			Description: "Three-suite apartment with a 40 m2 living room, integrated gourmet balcony and smart-home wiring throughout.",
			PriceUSD:    1150000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 3, Bathrooms: 3, AreaM2: 148, Floor: 21,
			District: "Leblon", City: "Rio de Janeiro", YearBuilt: 2021, Parking: 2, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1613490493576-7fde63acd811?w=1200",
		},
		{
			SKU: "KS-DEMO-REALTY-LS-2301", ComplexRef: "leblon-sky", ComplexName: "Leblon Sky Towers", Unit: "Penthouse 2301",
			Name:        "Leblon Sky — Penthouse 2301",
			Description: "Two-level penthouse: private rooftop with a plunge pool and outdoor kitchen, city-to-sea views from every room.",
			PriceUSD:    2300000, DealType: "sale", PropertyType: "penthouse",
			Bedrooms: 4, Bathrooms: 4, AreaM2: 240, Floor: 23,
			District: "Leblon", City: "Rio de Janeiro", YearBuilt: 2021, Parking: 3, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1600596542815-ffad4c1539a9?w=1200",
		},

		// ── Group 4: Barra Green Residences (2 units) ──
		{
			SKU: "KS-DEMO-REALTY-BG-0304", ComplexRef: "barra-green", ComplexName: "Barra Green Residences", Unit: "Unit 304",
			Name:        "Barra Green — Unit 304",
			Description: "Two-bedroom facing the condominium park, ground-level play area and a service room. Ten minutes from the international school.",
			PriceUSD:    214000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 2, Bathrooms: 2, AreaM2: 88, Floor: 3,
			District: "Barra da Tijuca", City: "Rio de Janeiro", YearBuilt: 2014, Parking: 1, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1567767292278-a4f21aa2d36e?w=1200",
		},
		{
			SKU: "KS-DEMO-REALTY-BG-0801", ComplexRef: "barra-green", ComplexName: "Barra Green Residences", Unit: "Unit 801",
			Name:        "Barra Green — Unit 801",
			Description: "Three-bedroom with two balconies, one suite and a laundry room. Two parking spaces and a storage cage included.",
			PriceUSD:    268000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 3, Bathrooms: 2, AreaM2: 118, Floor: 8,
			District: "Barra da Tijuca", City: "Rio de Janeiro", YearBuilt: 2014, Parking: 2, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1598928506311-c55ded91a20c?w=1200",
		},

		// ── Standalone objects (7) ──
		{
			SKU:         "KS-DEMO-REALTY-RJ-GAVEA-01",
			Name:        "Garden House in Gávea",
			Description: "Single-family house with a private garden, mango trees and a covered veranda. Quiet street five minutes from the park trails.",
			PriceUSD:    690000, DealType: "sale", PropertyType: "house",
			Bedrooms: 4, Bathrooms: 3, AreaM2: 240, Floor: 0,
			District: "Gávea", City: "Rio de Janeiro", YearBuilt: 2009, Parking: 2, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1568605114967-8130f3a36994?w=1200",
		},
		{
			SKU:         "KS-DEMO-REALTY-RJ-COPA-02",
			Name:        "Compact Studio near the Metro",
			Description: "Efficient studio two blocks from the metro. Built-in wardrobe, induction kitchen, fast fiber. Ideal first purchase or rental investment.",
			PriceUSD:    118000, DealType: "sale", PropertyType: "studio",
			Bedrooms: 1, Bathrooms: 1, AreaM2: 34, Floor: 7,
			District: "Copacabana", City: "Rio de Janeiro", YearBuilt: 2019, Parking: 0, Furnished: true,
			ImageURL: "https://images.unsplash.com/photo-1586023492125-27b2c045efd7?w=1200",
		},
		{
			SKU:         "KS-DEMO-REALTY-RJ-SANTO-03",
			Name:        "Loft in a Converted Warehouse",
			Description: "Industrial loft with 5 m ceilings, exposed brick and steel beams. Freight-elevator building with a co-work lobby.",
			PriceUSD:    245000, DealType: "sale", PropertyType: "loft",
			Bedrooms: 1, Bathrooms: 1, AreaM2: 95, Floor: 3,
			District: "Santo Cristo", City: "Rio de Janeiro", YearBuilt: 2018, Parking: 1, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1536376072261-38c75010e6c9?w=1200",
		},
		{
			SKU:         "KS-DEMO-REALTY-RJ-IPAN-04",
			Name:        "Seafront One-Bedroom for Rent",
			Description: "Furnished one-bedroom directly on the beachfront promenade. Monthly rent, bills included, minimum six months.",
			PriceUSD:    2400, DealType: "rent", PropertyType: "apartment",
			Bedrooms: 1, Bathrooms: 1, AreaM2: 58, Floor: 9,
			District: "Ipanema", City: "Rio de Janeiro", YearBuilt: 2012, Parking: 1, Furnished: true,
			ImageURL: "https://images.unsplash.com/photo-1560448075-bb485b067938?w=1200",
		},
		{
			SKU:         "KS-DEMO-REALTY-RJ-URCA-05",
			Name:        "Renovated Colonial Three-Bedroom",
			Description: "Colonial-era apartment fully renovated: restored hardwood floors, new plumbing and wiring, original stucco ceilings.",
			PriceUSD:    455000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 3, Bathrooms: 2, AreaM2: 140, Floor: 2,
			District: "Urca", City: "Rio de Janeiro", YearBuilt: 1938, Parking: 1, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1615529182904-14819c35db37?w=1200",
		},
		{
			SKU:         "KS-DEMO-REALTY-RJ-JOA-06",
			Name:        "Hillside Villa with Pool",
			Description: "Gated hillside villa: infinity pool, guest suite, outdoor cinema wall. Panoramic forest and sea views.",
			PriceUSD:    980000, DealType: "sale", PropertyType: "villa",
			Bedrooms: 5, Bathrooms: 5, AreaM2: 380, Floor: 0,
			District: "Joá", City: "Rio de Janeiro", YearBuilt: 2011, Parking: 4, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1580587771525-78b9dba3b914?w=1200",
		},
		{
			SKU:         "KS-DEMO-REALTY-RJ-FLAM-07",
			Name:        "Quiet Two-Bedroom with Home Office",
			Description: "Two-bedroom on a tree-lined block: the second room is set up as a studio, the living room opens onto an integrated kitchen. Building has a rooftop laundry and bike storage.",
			PriceUSD:    315000, DealType: "sale", PropertyType: "apartment",
			Bedrooms: 2, Bathrooms: 2, AreaM2: 98, Floor: 11,
			District: "Flamengo", City: "Rio de Janeiro", YearBuilt: 2017, Parking: 1, Furnished: false,
			ImageURL: "https://images.unsplash.com/photo-1554995207-c18c203602cb?w=1200",
		},
	}
}

// realtyLeads: 8 leads across the whole pipeline, so the CRM's first render
// shows a working funnel rather than a stack of identical "new" rows.
// Times are relative — past for what already happened, near-future for what
// is booked.
func realtyLeads() []DemoLead {
	return []DemoLead{
		{
			Key: "lead-01", Name: "Maria Santos", Phone: "+5521999990101", Email: "maria.santos@example.com",
			Message: "Interested in Vista Marina unit 1104. Asked about the building gym and a second parking space.",
			Status:  "new", DaysAhead: 2, Hour: 11,
		},
		{
			Key: "lead-02", Name: "Rafael Lima", Phone: "+5521999990102", Email: "rafael.lima@example.com",
			Message: "Wants a Saturday viewing at Leblon Sky unit 1902. Buying with a mortgage, pre-approved.",
			Status:  "new", DaysAhead: 4, Hour: 10,
		},
		{
			Key: "lead-03", Name: "Diego Rocha", Phone: "+5521999990103", Email: "diego.rocha@example.com",
			Message: "Asked for the floor plan of the Leblon Sky penthouse and the condo fee.",
			Status:  "new", DaysAhead: 5, Hour: 9,
		},
		{
			Key: "lead-04", Name: "Camila Duarte", Phone: "+5521999990104", Email: "camila.duarte@example.com",
			Message: "Called about the Gávea garden house; comparing it with the hillside villa in Joá.",
			Status:  "contacted", DaysAhead: -1, Hour: 16,
		},
		{
			Key: "lead-05", Name: "Tiago Ferreira", Phone: "+5521999990105", Email: "tiago.ferreira@example.com",
			Message: "Relocating in March, looking at Barra Green unit 801. Needs two parking spaces.",
			Status:  "contacted", DaysAhead: 6, Hour: 17,
		},
		{
			Key: "lead-06", Name: "Beatriz Moreira", Phone: "+5521999990106", Email: "beatriz.moreira@example.com",
			Message: "Viewing booked for the Ipanema seafront rental. Wants to move in next month.",
			Status:  "showing_booked", DaysAhead: 1, Hour: 15,
		},
		{
			Key: "lead-07", Name: "Lucas Almeida", Phone: "+5521999990107", Email: "lucas.almeida@example.com",
			Message: "Second viewing at Laranjeiras Park unit 502, bringing his partner and a contractor.",
			Status:  "showing_booked", DaysAhead: 3, Hour: 13,
		},
		{
			Key: "lead-08", Name: "Helena Costa", Phone: "+5521999990108", Email: "helena.costa@example.com",
			Message: "Signed for the Urca colonial three-bedroom. Keep in touch for referrals.",
			Status:  "closed", DaysAhead: -9, Hour: 12,
		},
	}
}

// realtyContacts: the people around the deals — buyers, an owner, a partner
// agent (V2_SPEC §4: the storefront URL is shared with buyers AND partner
// realtors).
func realtyContacts() []DemoContact {
	return []DemoContact{
		{
			Key: "contact-01", Name: "Ana Pereira", Phone: "+5521999990201", Email: "ana.pereira@example.com",
			Role: "buyer", Note: "Cash buyer up to 400,000 USD. Prefers Botafogo and Flamengo, no ground floors.",
		},
		{
			Key: "contact-02", Name: "Marcelo Nunes", Phone: "+5521999990202", Email: "marcelo.nunes@example.com",
			Role: "owner", Note: "Owner of the Urca colonial apartment. Wants a quiet sale, no open houses.",
		},
		{
			Key: "contact-03", Name: "Patricia Gomes", Phone: "+5521999990203", Email: "patricia.gomes@example.com",
			Role: "partner agent", Note: "Partner agent in Barra. Shares buyers for the Barra Green units.",
		},
		{
			Key: "contact-04", Name: "Bruno Tavares", Phone: "+5521999990204", Email: "bruno.tavares@example.com",
			Role: "buyer", Note: "Investor looking for rentals with a 6% yield. Interested in the studio near the metro.",
		},
	}
}
