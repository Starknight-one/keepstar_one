package postgres

// seedShoesProducts returns ~110 shoe master products.
func seedShoesProducts() []mp {
	return []mp{
		// ────────────────────────────────────────────────────────────────────
		// 1. Running Shoes (40 products)
		// ────────────────────────────────────────────────────────────────────
		// Nike (5)
		{
			sku:       "NIKE-PEGASUS-41",
			name:      "Nike Air Zoom Pegasus 41",
			desc:      "Legendary responsive running shoe with React foam and Zoom Air units for daily training.",
			brand:     "Nike",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/White","material":"Mesh","sole":"React Foam"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-VOMERO-17",
			name:      "Nike Vomero 17",
			desc:      "Maximum cushioning for long distance runs with plush ZoomX foam and soft upper.",
			brand:     "Nike",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Navy/Orange","material":"Mesh","sole":"ZoomX Foam"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-INFINITYRUN-4",
			name:      "Nike InfinityRun 4",
			desc:      "Injury prevention focused design with stable React platform for everyday running.",
			brand:     "Nike",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Grey/Lime","material":"Mesh","sole":"React Foam"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-ZOOMFLY-6",
			name:      "Nike Zoom Fly 6",
			desc:      "Tempo trainer with carbon-infused plate and ZoomX foam for speed workouts.",
			brand:     "Nike",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Volt/Black","material":"Mesh","sole":"ZoomX Foam"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-ALPHAFLY-3",
			name:      "Nike Alphafly 3",
			desc:      "Elite racing shoe with full-length carbon plate and dual Zoom Air pods for marathon performance.",
			brand:     "Nike",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Pink/White","material":"Mesh","sole":"ZoomX with Carbon Plate"}`,
			ownerSlug: "",
		},

		// Adidas (5)
		{
			sku:       "ADIDAS-ULTRABOOST-24",
			name:      "Adidas Ultraboost 24",
			desc:      "Iconic energy-returning running shoe with Boost midsole and Primeknit upper.",
			brand:     "Adidas",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Core Black","material":"Mesh","sole":"Boost"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-SUPERNOVA-RISE",
			name:      "Adidas Supernova Rise",
			desc:      "Supportive daily trainer with Dreamstrike+ cushioning for comfortable long runs.",
			brand:     "Adidas",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/Silver","material":"Mesh","sole":"Dreamstrike+"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-ADIZERO-SL2",
			name:      "Adidas Adizero SL2",
			desc:      "Lightweight speed shoe with Lightstrike EVA for fast training sessions.",
			brand:     "Adidas",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Solar Red","material":"Mesh","sole":"Lightstrike"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-BOSTON-12",
			name:      "Adidas Adizero Boston 12",
			desc:      "Versatile uptempo trainer with EnergyRods 2.0 for propulsive energy return.",
			brand:     "Adidas",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Orange/Black","material":"Mesh","sole":"Lightstrike Pro"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-ADIOSPRO-3",
			name:      "Adidas Adizero Adios Pro 3",
			desc:      "Elite marathon racer with five EnergyRods and Lightstrike Pro foam.",
			brand:     "Adidas",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Blue","material":"Mesh","sole":"Lightstrike Pro"}`,
			ownerSlug: "",
		},

		// New Balance (5)
		{
			sku:       "NB-1080V14",
			name:      "New Balance Fresh Foam 1080v14",
			desc:      "Premium neutral running shoe with plush Fresh Foam X cushioning.",
			brand:     "New Balance",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Grey/Yellow","material":"Mesh","sole":"Fresh Foam X"}`,
			ownerSlug: "",
		},
		{
			sku:       "NB-REBEL-V4",
			name:      "New Balance FuelCell Rebel v4",
			desc:      "Lightweight tempo shoe with bouncy FuelCell foam for fast training.",
			brand:     "New Balance",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/Pink","material":"Mesh","sole":"FuelCell"}`,
			ownerSlug: "",
		},
		{
			sku:       "NB-MORE-V5",
			name:      "New Balance Fresh Foam More v5",
			desc:      "Maximum cushioning with oversized Fresh Foam midsole for ultra-soft comfort.",
			brand:     "New Balance",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/White","material":"Mesh","sole":"Fresh Foam X"}`,
			ownerSlug: "",
		},
		{
			sku:       "NB-SCELITE-V4",
			name:      "New Balance FuelCell SC Elite v4",
			desc:      "Carbon-plated racing shoe with Energy Arc and FuelCell for elite performance.",
			brand:     "New Balance",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Lime/Black","material":"Mesh","sole":"FuelCell with Carbon Plate"}`,
			ownerSlug: "",
		},
		{
			sku:       "NB-880V14",
			name:      "New Balance 880v14",
			desc:      "Reliable daily trainer with balanced cushioning and durable construction.",
			brand:     "New Balance",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Navy/Orange","material":"Mesh","sole":"Fresh Foam X"}`,
			ownerSlug: "",
		},

		// Asics (5)
		{
			sku:       "ASICS-KAYANO-31",
			name:      "Asics Gel-Kayano 31",
			desc:      "Premium stability shoe with FF Blast Plus Eco and 4D Guidance System.",
			brand:     "Asics",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/Red","material":"Mesh","sole":"FF Blast Plus"}`,
			ownerSlug: "",
		},
		{
			sku:       "ASICS-NIMBUS-26",
			name:      "Asics Gel-Nimbus 26",
			desc:      "Luxurious neutral cushioning with FF Blast Plus Eco and PureGel technology.",
			brand:     "Asics",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/White","material":"Mesh","sole":"FF Blast Plus Eco"}`,
			ownerSlug: "",
		},
		{
			sku:       "ASICS-CUMULUS-26",
			name:      "Asics Gel-Cumulus 26",
			desc:      "Versatile daily trainer with balanced FF Blast Plus cushioning.",
			brand:     "Asics",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Grey/Green","material":"Mesh","sole":"FF Blast Plus"}`,
			ownerSlug: "",
		},
		{
			sku:       "ASICS-NOVABLAST-4",
			name:      "Asics Novablast 4",
			desc:      "Energetic trainer with trampoline-inspired FF Blast Plus Eco foam.",
			brand:     "Asics",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Orange/Black","material":"Mesh","sole":"FF Blast Plus Eco"}`,
			ownerSlug: "",
		},
		{
			sku:       "ASICS-METASPEED-SKY",
			name:      "Asics Metaspeed Sky+",
			desc:      "Elite racing shoe with carbon plate and FF Turbo foam for marathon performance.",
			brand:     "Asics",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Yellow/Blue","material":"Mesh","sole":"FF Turbo with Carbon Plate"}`,
			ownerSlug: "",
		},

		// Puma (5)
		{
			sku:       "PUMA-VELOCITY-3",
			name:      "Puma Velocity Nitro 3",
			desc:      "Daily trainer with responsive Nitro foam and PWRPLATE for smooth transitions.",
			brand:     "Puma",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/Yellow","material":"Mesh","sole":"Nitro Foam"}`,
			ownerSlug: "",
		},
		{
			sku:       "PUMA-DEVIATE-3",
			name:      "Puma Deviate Nitro 3",
			desc:      "Carbon-plated trainer with Nitro Elite foam for race-day speed.",
			brand:     "Puma",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Red/White","material":"Mesh","sole":"Nitro Elite"}`,
			ownerSlug: "",
		},
		{
			sku:       "PUMA-FASTR-ELITE-2",
			name:      "Puma Fast-R Nitro Elite 2",
			desc:      "Elite racing flat with PWRPLATE and Nitro Elite for 5K-10K races.",
			brand:     "Puma",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Green/Black","material":"Mesh","sole":"Nitro Elite"}`,
			ownerSlug: "",
		},
		{
			sku:       "PUMA-MAGNIFY-2",
			name:      "Puma Magnify Nitro 2",
			desc:      "Supportive cushioning with Nitro foam for stable daily training.",
			brand:     "Puma",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/Orange","material":"Mesh","sole":"Nitro Foam"}`,
			ownerSlug: "",
		},
		{
			sku:       "PUMA-RUNXX",
			name:      "Puma Run XX Nitro",
			desc:      "Women-specific running shoe with adaptive fit and responsive Nitro cushioning.",
			brand:     "Puma",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Pink/Purple","material":"Mesh","sole":"Nitro Foam"}`,
			ownerSlug: "",
		},

		// Hoka (5)
		{
			sku:       "HOKA-CLIFTON-9",
			name:      "Hoka Clifton 9",
			desc:      "Lightweight daily trainer with signature Hoka cushioning and smooth ride.",
			brand:     "Hoka",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Blue","material":"Mesh","sole":"EVA Foam"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOKA-BONDI-8",
			name:      "Hoka Bondi 8",
			desc:      "Maximum cushioning shoe with soft EVA foam for ultra-comfortable runs.",
			brand:     "Hoka",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/White","material":"Mesh","sole":"EVA Foam"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOKA-MACH-6",
			name:      "Hoka Mach 6",
			desc:      "Lightweight tempo trainer with PROFLY+ midsole for speed workouts.",
			brand:     "Hoka",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Orange/Navy","material":"Mesh","sole":"PROFLY+"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOKA-SPEEDGOAT-6",
			name:      "Hoka Speedgoat 6",
			desc:      "Trail running shoe with aggressive traction and cushioned protection.",
			brand:     "Hoka",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Green/Grey","material":"Mesh","sole":"Vibram Megagrip"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOKA-CIELO-X1",
			name:      "Hoka Cielo X1",
			desc:      "Elite racing shoe with carbon fiber plate and PEBA foam for marathon speed.",
			brand:     "Hoka",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Red","material":"Mesh","sole":"PEBA with Carbon Plate"}`,
			ownerSlug: "",
		},

		// Brooks (5)
		{
			sku:       "BROOKS-GHOST-16",
			name:      "Brooks Ghost 16",
			desc:      "Versatile neutral shoe with DNA Loft v2 for smooth daily training.",
			brand:     "Brooks",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/Grey","material":"Mesh","sole":"DNA Loft v2"}`,
			ownerSlug: "",
		},
		{
			sku:       "BROOKS-GLYCERIN-21",
			name:      "Brooks Glycerin 21",
			desc:      "Plush premium cushioning with DNA Loft v3 for luxurious comfort.",
			brand:     "Brooks",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/White","material":"Mesh","sole":"DNA Loft v3"}`,
			ownerSlug: "",
		},
		{
			sku:       "BROOKS-GTS-24",
			name:      "Brooks Adrenaline GTS 24",
			desc:      "Trusted stability shoe with GuideRails support for efficient stride.",
			brand:     "Brooks",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Navy/Orange","material":"Mesh","sole":"DNA Loft v2"}`,
			ownerSlug: "",
		},
		{
			sku:       "BROOKS-HYPERION-MAX2",
			name:      "Brooks Hyperion Max 2",
			desc:      "Lightweight racer with DNA Flash foam for fast marathon training.",
			brand:     "Brooks",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Blue","material":"Mesh","sole":"DNA Flash"}`,
			ownerSlug: "",
		},
		{
			sku:       "BROOKS-HYPERION-ELITE4",
			name:      "Brooks Hyperion Elite 4",
			desc:      "Elite carbon-plated racer with DNA Zero and Rapid Roll for race day.",
			brand:     "Brooks",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Orange/Black","material":"Mesh","sole":"DNA Zero with Carbon Plate"}`,
			ownerSlug: "",
		},

		// Saucony (5)
		{
			sku:       "SAUCONY-RIDE-17",
			name:      "Saucony Ride 17",
			desc:      "Balanced daily trainer with PWRRUN cushioning for versatile running.",
			brand:     "Saucony",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/Silver","material":"Mesh","sole":"PWRRUN"}`,
			ownerSlug: "",
		},
		{
			sku:       "SAUCONY-KINVARA-15",
			name:      "Saucony Kinvara 15",
			desc:      "Lightweight responsive trainer with PWRRUN foam for natural fast running.",
			brand:     "Saucony",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Green/Black","material":"Mesh","sole":"PWRRUN"}`,
			ownerSlug: "",
		},
		{
			sku:       "SAUCONY-SPEED-4",
			name:      "Saucony Endorphin Speed 4",
			desc:      "Tempo shoe with nylon plate and PWRRUN PB for race-pace training.",
			brand:     "Saucony",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Orange","material":"Mesh","sole":"PWRRUN PB"}`,
			ownerSlug: "",
		},
		{
			sku:       "SAUCONY-TRIUMPH-22",
			name:      "Saucony Triumph 22",
			desc:      "Maximum cushioning with thick PWRRUN+ midsole for plush comfort.",
			brand:     "Saucony",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Grey/Pink","material":"Mesh","sole":"PWRRUN+"}`,
			ownerSlug: "",
		},
		{
			sku:       "SAUCONY-ELITE-2",
			name:      "Saucony Endorphin Elite 2",
			desc:      "Elite carbon racer with PWRRUN HG foam for championship performance.",
			brand:     "Saucony",
			catSlug:   "running-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Yellow/Black","material":"Mesh","sole":"PWRRUN HG with Carbon Plate"}`,
			ownerSlug: "",
		},

		// ────────────────────────────────────────────────────────────────────
		// 2. Basketball Shoes (20 products)
		// ────────────────────────────────────────────────────────────────────
		// Nike (10)
		{
			sku:       "NIKE-LEBRON-22",
			name:      "Nike LeBron 22",
			desc:      "LeBron's signature shoe with Zoom Air Strobel and cushlon for power play.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Purple/Gold","material":"Synthetic","sole":"Zoom Air"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-KD-17",
			name:      "Nike KD 17",
			desc:      "Kevin Durant's signature with full-length Zoom Strobel for explosive moves.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/Red","material":"Synthetic","sole":"Zoom Air"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-GIANNIS-4",
			name:      "Nike Giannis Immortality 4",
			desc:      "Giannis signature shoe with responsive cushioning and multidirectional traction.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Green/Cream","material":"Synthetic","sole":"Cushlon"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-PG-7",
			name:      "Nike PG 7",
			desc:      "Paul George's signature with forefoot Air Zoom for quick cuts and lockdown.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/White","material":"Synthetic","sole":"Zoom Air"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-AJ38",
			name:      "Nike Air Jordan 38",
			desc:      "Latest Jordan signature with Formula 23 foam and Flight Plate for dynamic play.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/Red","material":"Synthetic","sole":"Formula 23"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-AJ1-LOW",
			name:      "Nike Air Jordan 1 Low",
			desc:      "Classic Jordan 1 silhouette in low-top for versatile on-court performance.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Red","material":"Synthetic","sole":"Air Cushioning"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-AJ4-RETRO",
			name:      "Nike Air Jordan 4 Retro",
			desc:      "Iconic Jordan 4 retro with visible Air and classic mesh panels.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Cement","material":"Synthetic","sole":"Air Cushioning"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-JA-2",
			name:      "Nike Ja 2",
			desc:      "Ja Morant signature shoe with Zoom Air and court-ready traction pattern.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Pink/Black","material":"Synthetic","sole":"Zoom Air"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-FREAK-6",
			name:      "Nike Zoom Freak 6",
			desc:      "Giannis Freak series with stacked Zoom Air for explosive performance.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Orange/Blue","material":"Synthetic","sole":"Zoom Air"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-BOOK-1",
			name:      "Nike Book 1",
			desc:      "Devin Booker signature with Zoom Air Strobel and secure lockdown fit.",
			brand:     "Nike",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Purple/Orange","material":"Synthetic","sole":"Zoom Air"}`,
			ownerSlug: "",
		},

		// Adidas (5)
		{
			sku:       "ADIDAS-HARDEN-8",
			name:      "Adidas Harden Vol 8",
			desc:      "James Harden signature with Boost cushioning and step-back support.",
			brand:     "Adidas",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/Gold","material":"Synthetic","sole":"Boost"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-DAME-9",
			name:      "Adidas Dame 9",
			desc:      "Damian Lillard signature with Bounce Pro cushioning and torsional support.",
			brand:     "Adidas",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Red/White","material":"Synthetic","sole":"Bounce Pro"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-AE1",
			name:      "Adidas AE 1",
			desc:      "Anthony Edwards signature debut with Boost and lightweight construction.",
			brand:     "Adidas",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/Yellow","material":"Synthetic","sole":"Boost"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-DON-6",
			name:      "Adidas D.O.N. Issue 6",
			desc:      "Donovan Mitchell signature with Lightstrike and dynamic traction.",
			brand:     "Adidas",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Green/Black","material":"Synthetic","sole":"Lightstrike"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-TRAE-3",
			name:      "Adidas Trae Young 3",
			desc:      "Trae Young signature with Lightstrike cushioning and lockdown fit.",
			brand:     "Adidas",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Red","material":"Synthetic","sole":"Lightstrike"}`,
			ownerSlug: "",
		},

		// Under Armour (3)
		{
			sku:       "UA-CURRY-12",
			name:      "Under Armour Curry 12",
			desc:      "Stephen Curry signature with UA Flow cushioning for elite court feel.",
			brand:     "Under Armour",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Gold/Blue","material":"Synthetic","sole":"UA Flow"}`,
			ownerSlug: "",
		},
		{
			sku:       "UA-CURRY-11",
			name:      "Under Armour Curry 11",
			desc:      "Previous Curry signature with UA Flow and anatomical upper.",
			brand:     "Under Armour",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/White","material":"Synthetic","sole":"UA Flow"}`,
			ownerSlug: "",
		},
		{
			sku:       "UA-HOVR-BREAKTHRU",
			name:      "Under Armour HOVR Breakthru",
			desc:      "Performance basketball shoe with HOVR cushioning and responsive feel.",
			brand:     "Under Armour",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Red/Black","material":"Synthetic","sole":"HOVR"}`,
			ownerSlug: "",
		},

		// Puma (2)
		{
			sku:       "PUMA-MB04",
			name:      "Puma MB.04",
			desc:      "LaMelo Ball signature with Nitro foam and space-age design.",
			brand:     "Puma",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Pink/Black","material":"Synthetic","sole":"Nitro Foam"}`,
			ownerSlug: "",
		},
		{
			sku:       "PUMA-RISE-NITRO",
			name:      "Puma Rise Nitro",
			desc:      "Performance basketball shoe with Nitro cushioning and stability features.",
			brand:     "Puma",
			catSlug:   "basketball-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/Orange","material":"Synthetic","sole":"Nitro Foam"}`,
			ownerSlug: "",
		},

		// ────────────────────────────────────────────────────────────────────
		// 3. Casual/Lifestyle Shoes (30 products)
		// ────────────────────────────────────────────────────────────────────
		// Nike (8)
		{
			sku:       "NIKE-AM95",
			name:      "Nike Air Max 95",
			desc:      "Iconic 90s runner with visible Air units and gradient upper design.",
			brand:     "Nike",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Grey/Neon","material":"Leather","sole":"Air Max"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-AM97",
			name:      "Nike Air Max 97",
			desc:      "Sleek 90s design with full-length Air unit and wavy upper lines.",
			brand:     "Nike",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Silver/Red","material":"Leather","sole":"Air Max"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-AM270",
			name:      "Nike Air Max 270",
			desc:      "Modern Air Max with large visible heel unit and breathable mesh.",
			brand:     "Nike",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/White","material":"Mesh","sole":"Air Max"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-AMPLUS",
			name:      "Nike Air Max Plus",
			desc:      "Bold Air Max design with Tuned Air technology and distinctive lines.",
			brand:     "Nike",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Blue/Orange","material":"Synthetic","sole":"Tuned Air"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-AM1",
			name:      "Nike Air Max 1",
			desc:      "The original Air Max with visible air cushioning and retro appeal.",
			brand:     "Nike",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Red","material":"Leather","sole":"Air Max"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-BLAZER-MID77",
			name:      "Nike Blazer Mid 77",
			desc:      "Vintage basketball-inspired sneaker with retro branding and clean lines.",
			brand:     "Nike",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Black","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-CORTEZ",
			name:      "Nike Cortez",
			desc:      "Classic running silhouette from 1972 with timeless design.",
			brand:     "Nike",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Red/Blue","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "NIKE-HUARACHE",
			name:      "Nike Air Huarache",
			desc:      "90s icon with neoprene bootie and heel cage for adaptive fit.",
			brand:     "Nike",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/White","material":"Neoprene","sole":"Air"}`,
			ownerSlug: "",
		},

		// Adidas (6)
		{
			sku:       "ADIDAS-STANSMITH",
			name:      "Adidas Stan Smith",
			desc:      "Minimalist tennis classic with perforated 3-Stripes and clean design.",
			brand:     "Adidas",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Green","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-GAZELLE",
			name:      "Adidas Gazelle",
			desc:      "Heritage suede sneaker with vintage appeal and 3-Stripes branding.",
			brand:     "Adidas",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Navy/White","material":"Suede","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-SAMBA-OG",
			name:      "Adidas Samba OG",
			desc:      "Football-inspired icon with gum sole and classic leather upper.",
			brand:     "Adidas",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/White","material":"Leather","sole":"Gum Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-FORUM-LOW",
			name:      "Adidas Forum Low",
			desc:      "Retro basketball sneaker with ankle strap and vintage vibes.",
			brand:     "Adidas",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Blue","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-NMD-R1",
			name:      "Adidas NMD R1",
			desc:      "Modern lifestyle sneaker with Boost cushioning and sock-like fit.",
			brand:     "Adidas",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/Red","material":"Mesh","sole":"Boost"}`,
			ownerSlug: "",
		},
		{
			sku:       "ADIDAS-SUPERSTAR",
			name:      "Adidas Superstar",
			desc:      "Hip-hop legend with iconic shell toe and 3-Stripes design.",
			brand:     "Adidas",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Black","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},

		// New Balance (5)
		{
			sku:       "NB-574",
			name:      "New Balance 574",
			desc:      "Classic lifestyle runner with ENCAP midsole and timeless silhouette.",
			brand:     "New Balance",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Grey/Navy","material":"Suede","sole":"ENCAP"}`,
			ownerSlug: "",
		},
		{
			sku:       "NB-550",
			name:      "New Balance 550",
			desc:      "Retro basketball sneaker revived with vintage styling and court appeal.",
			brand:     "New Balance",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Green","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "NB-2002R",
			name:      "New Balance 2002R",
			desc:      "Y2K-era design with N-ergy cushioning and layered construction.",
			brand:     "New Balance",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Grey/Silver","material":"Mesh","sole":"N-ergy"}`,
			ownerSlug: "",
		},
		{
			sku:       "NB-530",
			name:      "New Balance 530",
			desc:      "Chunky retro runner with ABZORB cushioning and 2000s aesthetic.",
			brand:     "New Balance",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Silver","material":"Mesh","sole":"ABZORB"}`,
			ownerSlug: "",
		},
		{
			sku:       "NB-327",
			name:      "New Balance 327",
			desc:      "70s-inspired design with oversized N and asymmetrical sole.",
			brand:     "New Balance",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Yellow/Grey","material":"Nylon","sole":"Rubber"}`,
			ownerSlug: "",
		},

		// Converse (3)
		{
			sku:       "CONV-CHUCKTAYLOR",
			name:      "Converse Chuck Taylor All Star",
			desc:      "Iconic canvas high-top with timeless design since 1917.",
			brand:     "Converse",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/White","material":"Canvas","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "CONV-CHUCK70",
			name:      "Converse Chuck 70",
			desc:      "Premium Chuck with vintage details and enhanced cushioning.",
			brand:     "Converse",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Parchment","material":"Canvas","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "CONV-ONESTAR",
			name:      "Converse One Star",
			desc:      "Low-top classic with suede upper and iconic star logo.",
			brand:     "Converse",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Green/White","material":"Suede","sole":"Rubber"}`,
			ownerSlug: "",
		},

		// Vans (3)
		{
			sku:       "VANS-OLDSKOOL",
			name:      "Vans Old Skool",
			desc:      "Skate classic with side stripe and durable canvas construction.",
			brand:     "Vans",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black/White","material":"Canvas","sole":"Waffle Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "VANS-SK8HI",
			name:      "Vans Sk8-Hi",
			desc:      "High-top skate shoe with padded collar and iconic side stripe.",
			brand:     "Vans",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Navy/White","material":"Canvas","sole":"Waffle Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "VANS-ERA",
			name:      "Vans Era",
			desc:      "Low-top skate shoe with padded collar for added comfort.",
			brand:     "Vans",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Red/White","material":"Canvas","sole":"Waffle Rubber"}`,
			ownerSlug: "",
		},

		// Reebok (2)
		{
			sku:       "REEBOK-CLASSIC",
			name:      "Reebok Classic Leather",
			desc:      "Soft leather classic with timeless silhouette and comfort.",
			brand:     "Reebok",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Gum","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "REEBOK-CLUBC85",
			name:      "Reebok Club C 85",
			desc:      "Minimalist tennis sneaker with vintage court appeal.",
			brand:     "Reebok",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Green","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},

		// Puma (2)
		{
			sku:       "PUMA-SUEDE",
			name:      "Puma Suede Classic",
			desc:      "Iconic suede sneaker with formstrip and timeless style since 1968.",
			brand:     "Puma",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Red/White","material":"Suede","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "PUMA-CALI",
			name:      "Puma Cali",
			desc:      "California-inspired sneaker with stacked sole and retro vibes.",
			brand:     "Puma",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"White/Black","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},

		// Asics (1)
		{
			sku:       "ASICS-GEL1130",
			name:      "Asics Gel-1130",
			desc:      "Y2K retro runner with GEL cushioning and layered design.",
			brand:     "Asics",
			catSlug:   "casual-shoes",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Silver/Blue","material":"Mesh","sole":"GEL"}`,
			ownerSlug: "",
		},

		// ────────────────────────────────────────────────────────────────────
		// 4. Boots & Sandals (20 products)
		// ────────────────────────────────────────────────────────────────────
		// Timberland (4)
		{
			sku:       "TIMB-6INCH",
			name:      "Timberland 6-Inch Premium Boot",
			desc:      "Iconic waterproof boot with premium nubuck leather and padded collar.",
			brand:     "Timberland",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Wheat","material":"Nubuck","sole":"Rubber Lug"}`,
			ownerSlug: "",
		},
		{
			sku:       "TIMB-EUROSPRINT",
			name:      "Timberland Euro Sprint",
			desc:      "Lightweight hiking boot with athletic styling and Timberland durability.",
			brand:     "Timberland",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "TIMB-GREYFIELD",
			name:      "Timberland Greyfield",
			desc:      "Modern leather boot with canvas details and OrthoLite comfort.",
			brand:     "Timberland",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Brown","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "TIMB-COURMA",
			name:      "Timberland Courma",
			desc:      "Versatile boot with premium leather and comfortable EK+ sole.",
			brand:     "Timberland",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Dark Brown","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},

		// Dr.Martens (4)
		{
			sku:       "DRMARTENS-1460",
			name:      "Dr.Martens 1460",
			desc:      "Iconic 8-eye boot with smooth leather and signature yellow stitching.",
			brand:     "Dr.Martens",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black","material":"Leather","sole":"AirWair"}`,
			ownerSlug: "",
		},
		{
			sku:       "DRMARTENS-1461",
			name:      "Dr.Martens 1461",
			desc:      "Classic 3-eye shoe with smooth leather and Docs DNA.",
			brand:     "Dr.Martens",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Cherry Red","material":"Leather","sole":"AirWair"}`,
			ownerSlug: "",
		},
		{
			sku:       "DRMARTENS-JADON",
			name:      "Dr.Martens Jadon",
			desc:      "Platform boot with chunky sole and rebellious style.",
			brand:     "Dr.Martens",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black","material":"Leather","sole":"Platform AirWair"}`,
			ownerSlug: "",
		},
		{
			sku:       "DRMARTENS-CHELSEA",
			name:      "Dr.Martens Chelsea",
			desc:      "Classic Chelsea boot with elastic sides and Docs durability.",
			brand:     "Dr.Martens",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black","material":"Leather","sole":"AirWair"}`,
			ownerSlug: "",
		},

		// Clarks (3)
		{
			sku:       "CLARKS-DESERT",
			name:      "Clarks Desert Boot",
			desc:      "Iconic suede chukka boot with crepe sole and timeless design since 1950.",
			brand:     "Clarks",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Sand Suede","material":"Suede","sole":"Crepe"}`,
			ownerSlug: "",
		},
		{
			sku:       "CLARKS-WALLABEE",
			name:      "Clarks Wallabee",
			desc:      "Moccasin-inspired shoe with crepe sole and laid-back style.",
			brand:     "Clarks",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Maple Suede","material":"Suede","sole":"Crepe"}`,
			ownerSlug: "",
		},
		{
			sku:       "CLARKS-DESERT-TREK",
			name:      "Clarks Desert Trek",
			desc:      "Hiking-inspired boot with suede upper and distinctive lacing.",
			brand:     "Clarks",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Beeswax","material":"Leather","sole":"Crepe"}`,
			ownerSlug: "",
		},

		// Ecco (3)
		{
			sku:       "ECCO-SOFT7",
			name:      "Ecco Soft 7",
			desc:      "Premium leather sneaker with exceptional comfort and Scandinavian design.",
			brand:     "Ecco",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black","material":"Leather","sole":"PU"}`,
			ownerSlug: "",
		},
		{
			sku:       "ECCO-HELSINKI2",
			name:      "Ecco Helsinki 2",
			desc:      "Versatile lace-up boot with GORE-TEX and comfortable fit.",
			brand:     "Ecco",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Brown","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "ECCO-TRACK25",
			name:      "Ecco Track 25",
			desc:      "Rugged outdoor boot with waterproof membrane and durable construction.",
			brand:     "Ecco",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Bison","material":"Leather","sole":"Rubber"}`,
			ownerSlug: "",
		},

		// UGG (2)
		{
			sku:       "UGG-CLASSIC",
			name:      "UGG Classic Short",
			desc:      "Iconic sheepskin boot with plush wool lining and cozy comfort.",
			brand:     "UGG",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Chestnut","material":"Sheepskin","sole":"Rubber"}`,
			ownerSlug: "",
		},
		{
			sku:       "UGG-NEUMEL",
			name:      "UGG Neumel",
			desc:      "Chukka boot with sheepskin lining and casual style.",
			brand:     "UGG",
			catSlug:   "boots",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black","material":"Suede","sole":"Rubber"}`,
			ownerSlug: "",
		},

		// Birkenstock (2) - sandals
		{
			sku:       "BIRK-BOSTON",
			name:      "Birkenstock Boston",
			desc:      "Classic clog with contoured cork footbed and adjustable strap.",
			brand:     "Birkenstock",
			catSlug:   "sandals",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Taupe Suede","material":"Suede","sole":"EVA"}`,
			ownerSlug: "",
		},
		{
			sku:       "BIRK-ARIZONA",
			name:      "Birkenstock Arizona",
			desc:      "Two-strap sandal with cork footbed and legendary comfort.",
			brand:     "Birkenstock",
			catSlug:   "sandals",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black","material":"Leather","sole":"EVA"}`,
			ownerSlug: "",
		},

		// Crocs (2) - sandals
		{
			sku:       "CROCS-CLASSIC",
			name:      "Crocs Classic Clog",
			desc:      "Iconic lightweight clog with Croslite foam and customizable Jibbitz.",
			brand:     "Crocs",
			catSlug:   "sandals",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Black","material":"Croslite","sole":"Croslite"}`,
			ownerSlug: "",
		},
		{
			sku:       "CROCS-LITERIDE",
			name:      "Crocs LiteRide",
			desc:      "Next-gen Crocs with LiteRide foam for enhanced comfort and style.",
			brand:     "Crocs",
			catSlug:   "sandals",
			images:    `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"]`,
			attrs:     `{"color":"Navy/White","material":"Croslite","sole":"LiteRide"}`,
			ownerSlug: "",
		},
	}
}

// seedShoesListings returns shoe listings.
func seedShoesListings() []listing {
	return []listing{
		// ────────────────────────────────────────────────────────────────────
		// 1. Running Shoes Listings
		// ────────────────────────────────────────────────────────────────────
		// Nike Running
		{tenantSlug: "nike", mpSKU: "NIKE-PEGASUS-41", price: 1399000, stock: 75, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-PEGASUS-41", price: 1259100, stock: 50, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "NIKE-VOMERO-17", price: 1599000, stock: 60, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-VOMERO-17", price: 1439100, stock: 40, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "NIKE-INFINITYRUN-4", price: 1499000, stock: 80, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-INFINITYRUN-4", price: 1349100, stock: 55, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "NIKE-ZOOMFLY-6", price: 1899000, stock: 45, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-ZOOMFLY-6", price: 1709100, stock: 30, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "NIKE-ALPHAFLY-3", price: 2699000, stock: 25, rating: 4.9},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-ALPHAFLY-3", price: 2429100, stock: 15, rating: 4.8},

		// Adidas Running
		{tenantSlug: "nike", mpSKU: "ADIDAS-ULTRABOOST-24", price: 1699000, stock: 70, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-ULTRABOOST-24", price: 1529100, stock: 60, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "ADIDAS-SUPERNOVA-RISE", price: 1299000, stock: 65, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-SUPERNOVA-RISE", price: 1169100, stock: 50, rating: 4.4},

		{tenantSlug: "nike", mpSKU: "ADIDAS-ADIZERO-SL2", price: 1199000, stock: 55, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-ADIZERO-SL2", price: 1079100, stock: 45, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "ADIDAS-BOSTON-12", price: 1599000, stock: 50, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-BOSTON-12", price: 1439100, stock: 40, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "ADIDAS-ADIOSPRO-3", price: 2299000, stock: 30, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-ADIOSPRO-3", price: 2069100, stock: 20, rating: 4.7},

		// New Balance Running
		{tenantSlug: "nike", mpSKU: "NB-1080V14", price: 1699000, stock: 65, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "NB-1080V14", price: 1529100, stock: 50, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "NB-REBEL-V4", price: 1399000, stock: 70, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "NB-REBEL-V4", price: 1259100, stock: 55, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "NB-MORE-V5", price: 1599000, stock: 55, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "NB-MORE-V5", price: 1439100, stock: 40, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "NB-SCELITE-V4", price: 2499000, stock: 25, rating: 4.9},
		{tenantSlug: "sportmaster", mpSKU: "NB-SCELITE-V4", price: 2249100, stock: 18, rating: 4.8},

		{tenantSlug: "nike", mpSKU: "NB-880V14", price: 1299000, stock: 80, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "NB-880V14", price: 1169100, stock: 65, rating: 4.4},

		// Asics Running
		{tenantSlug: "nike", mpSKU: "ASICS-KAYANO-31", price: 1899000, stock: 50, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "ASICS-KAYANO-31", price: 1709100, stock: 40, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "ASICS-NIMBUS-26", price: 1799000, stock: 60, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "ASICS-NIMBUS-26", price: 1619100, stock: 45, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "ASICS-CUMULUS-26", price: 1399000, stock: 70, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "ASICS-CUMULUS-26", price: 1259100, stock: 55, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "ASICS-NOVABLAST-4", price: 1499000, stock: 65, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "ASICS-NOVABLAST-4", price: 1349100, stock: 50, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "ASICS-METASPEED-SKY", price: 2499000, stock: 28, rating: 4.9},
		{tenantSlug: "sportmaster", mpSKU: "ASICS-METASPEED-SKY", price: 2249100, stock: 20, rating: 4.8},

		// Puma Running
		{tenantSlug: "nike", mpSKU: "PUMA-VELOCITY-3", price: 1099000, stock: 75, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "PUMA-VELOCITY-3", price: 989100, stock: 60, rating: 4.4},

		{tenantSlug: "nike", mpSKU: "PUMA-DEVIATE-3", price: 1599000, stock: 50, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "PUMA-DEVIATE-3", price: 1439100, stock: 40, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "PUMA-FASTR-ELITE-2", price: 1899000, stock: 35, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "PUMA-FASTR-ELITE-2", price: 1709100, stock: 25, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "PUMA-MAGNIFY-2", price: 1199000, stock: 65, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "PUMA-MAGNIFY-2", price: 1079100, stock: 50, rating: 4.4},

		{tenantSlug: "nike", mpSKU: "PUMA-RUNXX", price: 1299000, stock: 55, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "PUMA-RUNXX", price: 1169100, stock: 45, rating: 4.5},

		// Hoka Running
		{tenantSlug: "nike", mpSKU: "HOKA-CLIFTON-9", price: 1499000, stock: 80, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "HOKA-CLIFTON-9", price: 1349100, stock: 65, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "HOKA-BONDI-8", price: 1699000, stock: 60, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "HOKA-BONDI-8", price: 1529100, stock: 50, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "HOKA-MACH-6", price: 1399000, stock: 70, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "HOKA-MACH-6", price: 1259100, stock: 55, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "HOKA-SPEEDGOAT-6", price: 1599000, stock: 50, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "HOKA-SPEEDGOAT-6", price: 1439100, stock: 40, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "HOKA-CIELO-X1", price: 2599000, stock: 22, rating: 4.9},
		{tenantSlug: "sportmaster", mpSKU: "HOKA-CIELO-X1", price: 2339100, stock: 16, rating: 4.8},

		// Brooks Running
		{tenantSlug: "nike", mpSKU: "BROOKS-GHOST-16", price: 1399000, stock: 85, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "BROOKS-GHOST-16", price: 1259100, stock: 70, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "BROOKS-GLYCERIN-21", price: 1699000, stock: 65, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "BROOKS-GLYCERIN-21", price: 1529100, stock: 50, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "BROOKS-GTS-24", price: 1499000, stock: 75, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "BROOKS-GTS-24", price: 1349100, stock: 60, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "BROOKS-HYPERION-MAX2", price: 1799000, stock: 40, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "BROOKS-HYPERION-MAX2", price: 1619100, stock: 30, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "BROOKS-HYPERION-ELITE4", price: 2399000, stock: 25, rating: 4.9},
		{tenantSlug: "sportmaster", mpSKU: "BROOKS-HYPERION-ELITE4", price: 2159100, stock: 18, rating: 4.8},

		// Saucony Running
		{tenantSlug: "nike", mpSKU: "SAUCONY-RIDE-17", price: 1299000, stock: 80, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "SAUCONY-RIDE-17", price: 1169100, stock: 65, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "SAUCONY-KINVARA-15", price: 1199000, stock: 75, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "SAUCONY-KINVARA-15", price: 1079100, stock: 60, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "SAUCONY-SPEED-4", price: 1699000, stock: 55, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "SAUCONY-SPEED-4", price: 1529100, stock: 45, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "SAUCONY-TRIUMPH-22", price: 1599000, stock: 60, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "SAUCONY-TRIUMPH-22", price: 1439100, stock: 50, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "SAUCONY-ELITE-2", price: 2299000, stock: 28, rating: 4.9},
		{tenantSlug: "sportmaster", mpSKU: "SAUCONY-ELITE-2", price: 2069100, stock: 20, rating: 4.8},

		// ────────────────────────────────────────────────────────────────────
		// 2. Basketball Shoes Listings
		// ────────────────────────────────────────────────────────────────────
		// Nike Basketball
		{tenantSlug: "nike", mpSKU: "NIKE-LEBRON-22", price: 2099000, stock: 45, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-LEBRON-22", price: 1889100, stock: 35, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "NIKE-KD-17", price: 1799000, stock: 50, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-KD-17", price: 1619100, stock: 40, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "NIKE-GIANNIS-4", price: 1299000, stock: 60, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-GIANNIS-4", price: 1169100, stock: 50, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "NIKE-PG-7", price: 1399000, stock: 55, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-PG-7", price: 1259100, stock: 45, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "NIKE-AJ38", price: 2299000, stock: 35, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-AJ38", price: 2069100, stock: 25, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "NIKE-AJ1-LOW", price: 1199000, stock: 70, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-AJ1-LOW", price: 1079100, stock: 60, rating: 4.4},

		{tenantSlug: "nike", mpSKU: "NIKE-AJ4-RETRO", price: 1899000, stock: 40, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-AJ4-RETRO", price: 1709100, stock: 30, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "NIKE-JA-2", price: 1399000, stock: 55, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-JA-2", price: 1259100, stock: 45, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "NIKE-FREAK-6", price: 1299000, stock: 60, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-FREAK-6", price: 1169100, stock: 50, rating: 4.4},

		{tenantSlug: "nike", mpSKU: "NIKE-BOOK-1", price: 1599000, stock: 50, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-BOOK-1", price: 1439100, stock: 40, rating: 4.6},

		// Adidas Basketball
		{tenantSlug: "nike", mpSKU: "ADIDAS-HARDEN-8", price: 1799000, stock: 45, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-HARDEN-8", price: 1619100, stock: 35, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "ADIDAS-DAME-9", price: 1599000, stock: 50, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-DAME-9", price: 1439100, stock: 40, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "ADIDAS-AE1", price: 1399000, stock: 55, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-AE1", price: 1259100, stock: 45, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "ADIDAS-DON-6", price: 1299000, stock: 60, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-DON-6", price: 1169100, stock: 50, rating: 4.4},

		{tenantSlug: "nike", mpSKU: "ADIDAS-TRAE-3", price: 1199000, stock: 65, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-TRAE-3", price: 1079100, stock: 55, rating: 4.4},

		// Under Armour Basketball
		{tenantSlug: "nike", mpSKU: "UA-CURRY-12", price: 1899000, stock: 40, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "UA-CURRY-12", price: 1709100, stock: 30, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "UA-CURRY-11", price: 1699000, stock: 45, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "UA-CURRY-11", price: 1529100, stock: 35, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "UA-HOVR-BREAKTHRU", price: 1299000, stock: 55, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "UA-HOVR-BREAKTHRU", price: 1169100, stock: 45, rating: 4.4},

		// Puma Basketball
		{tenantSlug: "nike", mpSKU: "PUMA-MB04", price: 1399000, stock: 50, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "PUMA-MB04", price: 1259100, stock: 40, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "PUMA-RISE-NITRO", price: 1199000, stock: 60, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "PUMA-RISE-NITRO", price: 1079100, stock: 50, rating: 4.4},

		// ────────────────────────────────────────────────────────────────────
		// 3. Casual/Lifestyle Shoes Listings
		// ────────────────────────────────────────────────────────────────────
		// Nike Casual
		{tenantSlug: "nike", mpSKU: "NIKE-AM95", price: 1699000, stock: 65, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-AM95", price: 1529100, stock: 55, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "NIKE-AM95", price: 1699000, stock: 45, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "NIKE-AM97", price: 1899000, stock: 55, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-AM97", price: 1709100, stock: 45, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "NIKE-AM97", price: 1899000, stock: 35, rating: 4.7},

		{tenantSlug: "nike", mpSKU: "NIKE-AM270", price: 1499000, stock: 80, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-AM270", price: 1349100, stock: 65, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "NIKE-AM270", price: 1499000, stock: 50, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "NIKE-AMPLUS", price: 1799000, stock: 50, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-AMPLUS", price: 1619100, stock: 40, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "NIKE-AMPLUS", price: 1799000, stock: 30, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "NIKE-AM1", price: 1599000, stock: 70, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-AM1", price: 1439100, stock: 60, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "NIKE-AM1", price: 1599000, stock: 45, rating: 4.6},

		{tenantSlug: "nike", mpSKU: "NIKE-BLAZER-MID77", price: 1199000, stock: 75, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-BLAZER-MID77", price: 1079100, stock: 60, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "NIKE-BLAZER-MID77", price: 1199000, stock: 50, rating: 4.5},

		{tenantSlug: "nike", mpSKU: "NIKE-CORTEZ", price: 999000, stock: 90, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-CORTEZ", price: 899100, stock: 75, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "NIKE-CORTEZ", price: 999000, stock: 60, rating: 4.4},

		{tenantSlug: "nike", mpSKU: "NIKE-HUARACHE", price: 1299000, stock: 65, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "NIKE-HUARACHE", price: 1169100, stock: 55, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "NIKE-HUARACHE", price: 1299000, stock: 40, rating: 4.5},

		// Adidas Casual
		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-STANSMITH", price: 899000, stock: 100, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "ADIDAS-STANSMITH", price: 999000, stock: 80, rating: 4.6},

		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-GAZELLE", price: 949000, stock: 85, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "ADIDAS-GAZELLE", price: 1049000, stock: 70, rating: 4.5},

		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-SAMBA-OG", price: 1099000, stock: 90, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "ADIDAS-SAMBA-OG", price: 1199000, stock: 75, rating: 4.7},

		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-FORUM-LOW", price: 1049000, stock: 70, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "ADIDAS-FORUM-LOW", price: 1149000, stock: 55, rating: 4.4},

		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-NMD-R1", price: 1299000, stock: 75, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "ADIDAS-NMD-R1", price: 1399000, stock: 60, rating: 4.5},

		{tenantSlug: "sportmaster", mpSKU: "ADIDAS-SUPERSTAR", price: 899000, stock: 95, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "ADIDAS-SUPERSTAR", price: 999000, stock: 80, rating: 4.5},

		// New Balance Casual
		{tenantSlug: "sportmaster", mpSKU: "NB-574", price: 899000, stock: 90, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "NB-574", price: 999000, stock: 75, rating: 4.4},

		{tenantSlug: "sportmaster", mpSKU: "NB-550", price: 1299000, stock: 70, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "NB-550", price: 1399000, stock: 60, rating: 4.6},

		{tenantSlug: "sportmaster", mpSKU: "NB-2002R", price: 1499000, stock: 65, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "NB-2002R", price: 1599000, stock: 50, rating: 4.6},

		{tenantSlug: "sportmaster", mpSKU: "NB-530", price: 1099000, stock: 80, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "NB-530", price: 1199000, stock: 65, rating: 4.4},

		{tenantSlug: "sportmaster", mpSKU: "NB-327", price: 999000, stock: 85, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "NB-327", price: 1099000, stock: 70, rating: 4.5},

		// Converse Casual
		{tenantSlug: "sportmaster", mpSKU: "CONV-CHUCKTAYLOR", price: 599000, stock: 100, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "CONV-CHUCKTAYLOR", price: 649000, stock: 90, rating: 4.4},

		{tenantSlug: "sportmaster", mpSKU: "CONV-CHUCK70", price: 799000, stock: 80, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "CONV-CHUCK70", price: 849000, stock: 70, rating: 4.5},

		{tenantSlug: "sportmaster", mpSKU: "CONV-ONESTAR", price: 699000, stock: 75, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "CONV-ONESTAR", price: 749000, stock: 65, rating: 4.3},

		// Vans Casual
		{tenantSlug: "sportmaster", mpSKU: "VANS-OLDSKOOL", price: 699000, stock: 95, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "VANS-OLDSKOOL", price: 749000, stock: 80, rating: 4.5},

		{tenantSlug: "sportmaster", mpSKU: "VANS-SK8HI", price: 799000, stock: 85, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "VANS-SK8HI", price: 849000, stock: 70, rating: 4.5},

		{tenantSlug: "sportmaster", mpSKU: "VANS-ERA", price: 649000, stock: 90, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "VANS-ERA", price: 699000, stock: 75, rating: 4.4},

		// Reebok Casual
		{tenantSlug: "sportmaster", mpSKU: "REEBOK-CLASSIC", price: 799000, stock: 85, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "REEBOK-CLASSIC", price: 849000, stock: 70, rating: 4.4},

		{tenantSlug: "sportmaster", mpSKU: "REEBOK-CLUBC85", price: 799000, stock: 80, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "REEBOK-CLUBC85", price: 849000, stock: 65, rating: 4.5},

		// Puma Casual
		{tenantSlug: "sportmaster", mpSKU: "PUMA-SUEDE", price: 799000, stock: 85, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "PUMA-SUEDE", price: 849000, stock: 70, rating: 4.5},

		{tenantSlug: "sportmaster", mpSKU: "PUMA-CALI", price: 899000, stock: 75, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "PUMA-CALI", price: 949000, stock: 60, rating: 4.4},

		// Asics Casual
		{tenantSlug: "sportmaster", mpSKU: "ASICS-GEL1130", price: 999000, stock: 70, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "ASICS-GEL1130", price: 1049000, stock: 55, rating: 4.5},

		// ────────────────────────────────────────────────────────────────────
		// 4. Boots & Sandals Listings
		// ────────────────────────────────────────────────────────────────────
		// Timberland Boots
		{tenantSlug: "sportmaster", mpSKU: "TIMB-6INCH", price: 2199000, stock: 60, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "TIMB-6INCH", price: 2299000, stock: 50, rating: 4.7},

		{tenantSlug: "sportmaster", mpSKU: "TIMB-EUROSPRINT", price: 1799000, stock: 55, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "TIMB-EUROSPRINT", price: 1899000, stock: 45, rating: 4.5},

		{tenantSlug: "sportmaster", mpSKU: "TIMB-GREYFIELD", price: 1699000, stock: 50, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "TIMB-GREYFIELD", price: 1799000, stock: 40, rating: 4.4},

		{tenantSlug: "sportmaster", mpSKU: "TIMB-COURMA", price: 1599000, stock: 60, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "TIMB-COURMA", price: 1699000, stock: 50, rating: 4.5},

		// Dr.Martens Boots
		{tenantSlug: "sportmaster", mpSKU: "DRMARTENS-1460", price: 1899000, stock: 70, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "DRMARTENS-1460", price: 1999000, stock: 60, rating: 4.6},

		{tenantSlug: "sportmaster", mpSKU: "DRMARTENS-1461", price: 1699000, stock: 65, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "DRMARTENS-1461", price: 1799000, stock: 55, rating: 4.5},

		{tenantSlug: "sportmaster", mpSKU: "DRMARTENS-JADON", price: 2299000, stock: 45, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "DRMARTENS-JADON", price: 2399000, stock: 35, rating: 4.6},

		{tenantSlug: "sportmaster", mpSKU: "DRMARTENS-CHELSEA", price: 1799000, stock: 55, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "DRMARTENS-CHELSEA", price: 1899000, stock: 45, rating: 4.5},

		// Clarks Boots
		{tenantSlug: "sportmaster", mpSKU: "CLARKS-DESERT", price: 1299000, stock: 80, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "CLARKS-DESERT", price: 1399000, stock: 70, rating: 4.6},

		{tenantSlug: "sportmaster", mpSKU: "CLARKS-WALLABEE", price: 1399000, stock: 70, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "CLARKS-WALLABEE", price: 1499000, stock: 60, rating: 4.5},

		{tenantSlug: "sportmaster", mpSKU: "CLARKS-DESERT-TREK", price: 1499000, stock: 60, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "CLARKS-DESERT-TREK", price: 1599000, stock: 50, rating: 4.5},

		// Ecco Boots
		{tenantSlug: "sportmaster", mpSKU: "ECCO-SOFT7", price: 1799000, stock: 65, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "ECCO-SOFT7", price: 1899000, stock: 55, rating: 4.6},

		{tenantSlug: "sportmaster", mpSKU: "ECCO-HELSINKI2", price: 1999000, stock: 50, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "ECCO-HELSINKI2", price: 2099000, stock: 40, rating: 4.6},

		{tenantSlug: "sportmaster", mpSKU: "ECCO-TRACK25", price: 2199000, stock: 45, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "ECCO-TRACK25", price: 2299000, stock: 35, rating: 4.7},

		// UGG Boots
		{tenantSlug: "sportmaster", mpSKU: "UGG-CLASSIC", price: 2499000, stock: 60, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "UGG-CLASSIC", price: 2599000, stock: 50, rating: 4.7},

		{tenantSlug: "sportmaster", mpSKU: "UGG-NEUMEL", price: 1999000, stock: 55, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "UGG-NEUMEL", price: 2099000, stock: 45, rating: 4.6},

		// Birkenstock Sandals
		{tenantSlug: "sportmaster", mpSKU: "BIRK-BOSTON", price: 1499000, stock: 75, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "BIRK-BOSTON", price: 1599000, stock: 65, rating: 4.7},

		{tenantSlug: "sportmaster", mpSKU: "BIRK-ARIZONA", price: 1299000, stock: 80, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "BIRK-ARIZONA", price: 1399000, stock: 70, rating: 4.6},

		// Crocs Sandals
		{tenantSlug: "sportmaster", mpSKU: "CROCS-CLASSIC", price: 599000, stock: 100, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "CROCS-CLASSIC", price: 649000, stock: 85, rating: 4.4},

		{tenantSlug: "sportmaster", mpSKU: "CROCS-LITERIDE", price: 749000, stock: 90, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "CROCS-LITERIDE", price: 799000, stock: 75, rating: 4.5},
	}
}
