package postgres

// seedElecHomeProducts returns ~45 gaming, smart-home, and TV master products.
func seedElecHomeProducts() []mp {
	return []mp{
		// ── Gaming (15 products) ─────────────────────────────────────

		// Sony (4)
		{
			sku:       "GAME-PS5-DISC",
			name:      "Sony PlayStation 5",
			desc:      "Next-gen gaming console with ultra-high speed SSD, ray tracing, and 4K gaming at up to 120fps.",
			brand:     "Sony",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1606144042614-b2417e99c4e3?w=800"]`,
			attrs:     `{"color":"White","type":"console"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-PS5-DIGITAL",
			name:      "Sony PlayStation 5 Digital Edition",
			desc:      "All-digital PS5 console without a disc drive, offering the same powerful performance in a slimmer design.",
			brand:     "Sony",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1606144042614-b2417e99c4e3?w=800"]`,
			attrs:     `{"color":"White","type":"console"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-DUALSENSE",
			name:      "Sony DualSense Wireless Controller",
			desc:      "Immersive controller with haptic feedback and adaptive triggers for a revolutionary gaming experience.",
			brand:     "Sony",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1592840496694-26d035b52b48?w=800"]`,
			attrs:     `{"color":"White","type":"controller"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-PSVR2",
			name:      "Sony PlayStation VR2",
			desc:      "Next-generation VR headset with OLED display, eye tracking, and 3D audio for truly immersive gameplay.",
			brand:     "Sony",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1617802690992-15d93263d3a9?w=800"]`,
			attrs:     `{"color":"White","type":"vr"}`,
			ownerSlug: "",
		},

		// Microsoft (3)
		{
			sku:       "GAME-XBOX-X",
			name:      "Microsoft Xbox Series X",
			desc:      "The fastest, most powerful Xbox ever with 12 teraflops of processing power and true 4K gaming.",
			brand:     "Microsoft",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1621259182978-fbf93132d53d?w=800"]`,
			attrs:     `{"color":"Black","type":"console"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-XBOX-S",
			name:      "Microsoft Xbox Series S",
			desc:      "Compact all-digital console delivering next-gen speed and performance at an accessible price point.",
			brand:     "Microsoft",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1621259182978-fbf93132d53d?w=800"]`,
			attrs:     `{"color":"White","type":"console"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-XBOX-CTRL",
			name:      "Microsoft Xbox Wireless Controller",
			desc:      "Ergonomic wireless controller with textured grip and seamless compatibility across Xbox and PC.",
			brand:     "Microsoft",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1600080972464-8e5f35f63d08?w=800"]`,
			attrs:     `{"color":"Carbon Black","type":"controller"}`,
			ownerSlug: "",
		},

		// Nintendo (3)
		{
			sku:       "GAME-SWITCH-OLED",
			name:      "Nintendo Switch OLED Model",
			desc:      "Hybrid gaming console featuring a vibrant 7-inch OLED screen with enhanced audio and a wide adjustable stand.",
			brand:     "Nintendo",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1578303512597-81e6cc155b3e?w=800"]`,
			attrs:     `{"color":"White","type":"console"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-SWITCH-LITE",
			name:      "Nintendo Switch Lite",
			desc:      "Lightweight, compact handheld console optimized for personal portable play.",
			brand:     "Nintendo",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1578303512597-81e6cc155b3e?w=800"]`,
			attrs:     `{"color":"Turquoise","type":"console"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-NSW-PROCON",
			name:      "Nintendo Switch Pro Controller",
			desc:      "Premium wireless controller with motion controls, HD rumble, and amiibo functionality.",
			brand:     "Nintendo",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1585620385456-4a0a5e906fa1?w=800"]`,
			attrs:     `{"color":"Black","type":"controller"}`,
			ownerSlug: "",
		},

		// Valve (2)
		{
			sku:       "GAME-STEAMDECK-OLED",
			name:      "Valve Steam Deck OLED",
			desc:      "Handheld gaming PC with a stunning 7.4-inch HDR OLED display and extended battery life.",
			brand:     "Valve",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1640955014216-75201056c829?w=800"]`,
			attrs:     `{"color":"Black","type":"console"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-STEAMDECK-LCD",
			name:      "Valve Steam Deck LCD",
			desc:      "Portable PC gaming handheld with a custom APU, 7-inch LCD screen, and full SteamOS integration.",
			brand:     "Valve",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1640955014216-75201056c829?w=800"]`,
			attrs:     `{"color":"Black","type":"console"}`,
			ownerSlug: "",
		},

		// Razer (3)
		{
			sku:       "GAME-RAZER-WOLVERINE",
			name:      "Razer Wolverine V2 Chroma",
			desc:      "Tournament-grade wired controller with Razer Mecha-Tactile buttons and Chroma RGB lighting.",
			brand:     "Razer",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1600080972464-8e5f35f63d08?w=800"]`,
			attrs:     `{"color":"Black","type":"controller"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-RAZER-KISHI",
			name:      "Razer Kishi V2 Mobile Controller",
			desc:      "Universal mobile gaming controller with ultra-low latency connection and ergonomic design.",
			brand:     "Razer",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1600080972464-8e5f35f63d08?w=800"]`,
			attrs:     `{"color":"Black","type":"accessory"}`,
			ownerSlug: "",
		},
		{
			sku:       "GAME-RAZER-KAIRA",
			name:      "Razer Kaira Pro Wireless Headset",
			desc:      "Wireless gaming headset with TriForce Titanium 50mm drivers and HyperClear Supercardioid mic.",
			brand:     "Razer",
			catSlug:   "gaming",
			images:    `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=800"]`,
			attrs:     `{"color":"Black","type":"accessory"}`,
			ownerSlug: "",
		},

		// ── Smart Home (15 products) ─────────────────────────────────

		// Yandex (4)
		{
			sku:       "HOME-YA-STATION-MAX",
			name:      "Yandex Station Max",
			desc:      "Premium smart speaker with powerful stereo sound, LED display, and built-in Alice voice assistant.",
			brand:     "Yandex",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558089687-f282ffcbc126?w=800"]`,
			attrs:     `{"color":"Black","type":"speaker"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-YA-STATION-MINI",
			name:      "Yandex Station Mini",
			desc:      "Compact smart speaker with surprisingly rich sound quality and Alice voice assistant integration.",
			brand:     "Yandex",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558089687-f282ffcbc126?w=800"]`,
			attrs:     `{"color":"Grey","type":"speaker"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-YA-STATION-LITE",
			name:      "Yandex Station Lite",
			desc:      "Affordable entry-level smart speaker with Alice assistant for music, smart home control, and more.",
			brand:     "Yandex",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558089687-f282ffcbc126?w=800"]`,
			attrs:     `{"color":"White","type":"speaker"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-YA-SMART-BTN",
			name:      "Yandex Smart Button",
			desc:      "Wireless smart button for triggering scenes and routines in your Yandex smart home ecosystem.",
			brand:     "Yandex",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558002038-1055907df827?w=800"]`,
			attrs:     `{"color":"White","type":"sensor"}`,
			ownerSlug: "",
		},

		// Google (3)
		{
			sku:       "HOME-NEST-HUB2",
			name:      "Google Nest Hub 2nd Gen",
			desc:      "Smart display with 7-inch touchscreen, sleep sensing technology, and Google Assistant built in.",
			brand:     "Google",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558089687-f282ffcbc126?w=800"]`,
			attrs:     `{"color":"Chalk","type":"display"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-NEST-MINI",
			name:      "Google Nest Mini",
			desc:      "Compact smart speaker with improved bass and Google Assistant for hands-free help at home.",
			brand:     "Google",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558089687-f282ffcbc126?w=800"]`,
			attrs:     `{"color":"Charcoal","type":"speaker"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-CHROMECAST-4K",
			name:      "Google Chromecast with Google TV 4K",
			desc:      "Streaming device with dedicated remote, 4K HDR support, and personalized recommendations.",
			brand:     "Google",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1611532736597-de2d4265fba3?w=800"]`,
			attrs:     `{"color":"Snow","type":"display"}`,
			ownerSlug: "",
		},

		// Amazon (2)
		{
			sku:       "HOME-ECHO-DOT5",
			name:      "Amazon Echo Dot 5th Gen",
			desc:      "Most popular smart speaker with improved audio, temperature sensor, and Alexa voice control.",
			brand:     "Amazon",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1543512214-318228f32838?w=800"]`,
			attrs:     `{"color":"Charcoal","type":"speaker"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-ECHO-SHOW8",
			name:      "Amazon Echo Show 8",
			desc:      "Smart display with 8-inch HD screen, stereo speakers, and Alexa for video calls and entertainment.",
			brand:     "Amazon",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1543512214-318228f32838?w=800"]`,
			attrs:     `{"color":"Charcoal","type":"display"}`,
			ownerSlug: "",
		},

		// Xiaomi (3)
		{
			sku:       "HOME-MI-HUB",
			name:      "Xiaomi Mi Smart Home Hub",
			desc:      "Central hub for connecting and controlling Xiaomi smart home devices via Zigbee and Bluetooth.",
			brand:     "Xiaomi",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558002038-1055907df827?w=800"]`,
			attrs:     `{"color":"White","type":"sensor"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-MI-CAM-2K",
			name:      "Xiaomi Mi Camera 2K",
			desc:      "Indoor security camera with 2K resolution, night vision, and AI-powered human detection.",
			brand:     "Xiaomi",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1585771724684-38269d6639fd?w=800"]`,
			attrs:     `{"color":"White","type":"camera"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-MI-PURIFIER4",
			name:      "Xiaomi Mi Air Purifier 4",
			desc:      "High-efficiency air purifier with HEPA filter, OLED display, and smart app control for rooms up to 48 sqm.",
			brand:     "Xiaomi",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1585771724684-38269d6639fd?w=800"]`,
			attrs:     `{"color":"White","type":"sensor"}`,
			ownerSlug: "",
		},

		// Philips (3)
		{
			sku:       "HOME-HUE-STARTER",
			name:      "Philips Hue White and Color Starter Kit",
			desc:      "Smart lighting starter kit with 3 color bulbs, Hue Bridge, and dimmer switch for full smart control.",
			brand:     "Philips",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558002038-1055907df827?w=800"]`,
			attrs:     `{"color":"White","type":"light"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-HUE-LIGHTSTRIP",
			name:      "Philips Hue Lightstrip Plus V4",
			desc:      "Flexible LED lightstrip with 16 million colors, perfect for accent lighting and entertainment setups.",
			brand:     "Philips",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558002038-1055907df827?w=800"]`,
			attrs:     `{"color":"White","type":"light"}`,
			ownerSlug: "",
		},
		{
			sku:       "HOME-HUE-GO",
			name:      "Philips Hue Go Portable Light",
			desc:      "Portable smart light with rechargeable battery and millions of color options for any room or outdoors.",
			brand:     "Philips",
			catSlug:   "smart-home",
			images:    `["https://images.unsplash.com/photo-1558002038-1055907df827?w=800"]`,
			attrs:     `{"color":"White","type":"light"}`,
			ownerSlug: "",
		},

		// ── TVs (15 products) ────────────────────────────────────────

		// Samsung (4)
		{
			sku:       "TV-SAM-NQLED85",
			name:      "Samsung Neo QLED 8K 85\" QN900C",
			desc:      "Flagship 85-inch 8K TV with Neural Quantum Processor, Infinity Screen, and Dolby Atmos sound.",
			brand:     "Samsung",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Titan Black","size":"85 inch","type":"QLED","resolution":"8K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-SAM-NQLED65",
			name:      "Samsung Neo QLED 4K 65\" QN90C",
			desc:      "Premium 65-inch 4K QLED TV with Mini LED backlighting, 144Hz refresh rate, and anti-reflection technology.",
			brand:     "Samsung",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Titan Black","size":"65 inch","type":"QLED","resolution":"4K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-SAM-CRYSTAL55",
			name:      "Samsung Crystal UHD 55\" CU8000",
			desc:      "Slim 55-inch 4K TV with Crystal Processor, HDR support, and SmartThings hub integration.",
			brand:     "Samsung",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"55 inch","type":"LED","resolution":"4K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-SAM-CRYSTAL43",
			name:      "Samsung Crystal UHD 43\" CU7000",
			desc:      "Compact 43-inch 4K TV with PurColor technology and Crystal Processor 4K for vivid, lifelike picture.",
			brand:     "Samsung",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"43 inch","type":"LED","resolution":"4K"}`,
			ownerSlug: "",
		},

		// LG (4)
		{
			sku:       "TV-LG-OLEDC3-65",
			name:      "LG OLED C3 65\" evo",
			desc:      "65-inch OLED TV with self-lit pixels, a9 Gen6 AI Processor, and Dolby Vision IQ for stunning contrast.",
			brand:     "LG",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"65 inch","type":"OLED","resolution":"4K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-LG-OLEDB3-55",
			name:      "LG OLED B3 55\"",
			desc:      "55-inch OLED TV with perfect blacks, wide viewing angles, and webOS smart platform.",
			brand:     "LG",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"55 inch","type":"OLED","resolution":"4K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-LG-NANO55",
			name:      "LG NanoCell 55\" NANO76",
			desc:      "55-inch NanoCell TV with pure colors, a5 Gen6 AI Processor, and Game Optimizer for smooth gameplay.",
			brand:     "LG",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"55 inch","type":"LED","resolution":"4K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-LG-UHD43",
			name:      "LG UHD 43\" UR7800",
			desc:      "Budget-friendly 43-inch 4K UHD TV with AI Sound, HDR10, and LG ThinQ smart features.",
			brand:     "LG",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"43 inch","type":"LED","resolution":"4K"}`,
			ownerSlug: "",
		},

		// Sony (3)
		{
			sku:       "TV-SONY-A80L-65",
			name:      "Sony Bravia XR A80L 65\" OLED",
			desc:      "65-inch OLED TV with Cognitive Processor XR, XR Triluminos Pro, and Acoustic Surface Audio+ technology.",
			brand:     "Sony",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Titan Black","size":"65 inch","type":"OLED","resolution":"4K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-SONY-X90L-55",
			name:      "Sony Bravia X90L 55\" Full Array LED",
			desc:      "55-inch Full Array LED TV with XR Processor, XR Contrast Booster, and Google TV built in.",
			brand:     "Sony",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"55 inch","type":"LED","resolution":"4K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-SONY-X75L-50",
			name:      "Sony Bravia X75L 50\"",
			desc:      "Affordable 50-inch 4K TV with X1 processor, Motionflow XR, and Google TV smart platform.",
			brand:     "Sony",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"50 inch","type":"LED","resolution":"4K"}`,
			ownerSlug: "",
		},

		// TCL (2)
		{
			sku:       "TV-TCL-C845-65",
			name:      "TCL C845 65\" Mini LED",
			desc:      "65-inch Mini LED TV with QLED technology, 144Hz VRR, and Google TV for gaming and cinema enthusiasts.",
			brand:     "TCL",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"65 inch","type":"QLED","resolution":"4K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-TCL-P745-55",
			name:      "TCL P745 55\" 4K HDR",
			desc:      "Value-packed 55-inch 4K TV with HDR10+, Dolby Vision, and Google TV smart platform.",
			brand:     "TCL",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"55 inch","type":"LED","resolution":"4K"}`,
			ownerSlug: "",
		},

		// Hisense (2)
		{
			sku:       "TV-HIS-U8K-65",
			name:      "Hisense U8K 65\" Mini LED ULED",
			desc:      "65-inch Mini LED TV with quantum dot color, 2000 nits peak brightness, and 144Hz gaming mode.",
			brand:     "Hisense",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"65 inch","type":"QLED","resolution":"4K"}`,
			ownerSlug: "",
		},
		{
			sku:       "TV-HIS-A6K-55",
			name:      "Hisense A6K 55\" 4K UHD",
			desc:      "Affordable 55-inch 4K TV with Dolby Vision, DTS Virtual X, and VIDAA smart TV system.",
			brand:     "Hisense",
			catSlug:   "tvs",
			images:    `["https://images.unsplash.com/photo-1593359677879-a4bb92f829d1?w=800"]`,
			attrs:     `{"color":"Black","size":"55 inch","type":"LED","resolution":"4K"}`,
			ownerSlug: "",
		},
	}
}

// seedElecHomeListings returns ~90 gaming, smart-home, and TV listings.
func seedElecHomeListings() []listing {
	return []listing{
		// ── Gaming — Sony ────────────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "GAME-PS5-DISC", price: 4999000, stock: 15, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "GAME-PS5-DISC", price: 5199000, stock: 8, rating: 4.7},

		{tenantSlug: "techstore", mpSKU: "GAME-PS5-DIGITAL", price: 3999000, stock: 20, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "GAME-PS5-DIGITAL", price: 4099000, stock: 10, rating: 4.6},

		{tenantSlug: "techstore", mpSKU: "GAME-DUALSENSE", price: 699000, stock: 30, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "GAME-DUALSENSE", price: 729000, stock: 25, rating: 4.6},

		{tenantSlug: "techstore", mpSKU: "GAME-PSVR2", price: 5499000, stock: 10, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "GAME-PSVR2", price: 5699000, stock: 5, rating: 4.4},

		// ── Gaming — Microsoft ───────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "GAME-XBOX-X", price: 4999000, stock: 12, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "GAME-XBOX-X", price: 5199000, stock: 7, rating: 4.7},

		{tenantSlug: "techstore", mpSKU: "GAME-XBOX-S", price: 2999000, stock: 25, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "GAME-XBOX-S", price: 3099000, stock: 15, rating: 4.5},

		{tenantSlug: "techstore", mpSKU: "GAME-XBOX-CTRL", price: 599000, stock: 30, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "GAME-XBOX-CTRL", price: 619000, stock: 20, rating: 4.5},

		// ── Gaming — Nintendo ────────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "GAME-SWITCH-OLED", price: 2999000, stock: 20, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "GAME-SWITCH-OLED", price: 3099000, stock: 12, rating: 4.7},

		{tenantSlug: "techstore", mpSKU: "GAME-SWITCH-LITE", price: 1999000, stock: 25, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "GAME-SWITCH-LITE", price: 2099000, stock: 18, rating: 4.4},

		{tenantSlug: "techstore", mpSKU: "GAME-NSW-PROCON", price: 699000, stock: 20, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "GAME-NSW-PROCON", price: 729000, stock: 15, rating: 4.5},

		// ── Gaming — Valve ───────────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "GAME-STEAMDECK-OLED", price: 5499000, stock: 8, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "GAME-STEAMDECK-OLED", price: 5699000, stock: 5, rating: 4.7},

		{tenantSlug: "techstore", mpSKU: "GAME-STEAMDECK-LCD", price: 3999000, stock: 12, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "GAME-STEAMDECK-LCD", price: 4199000, stock: 7, rating: 4.5},

		// ── Gaming — Razer ───────────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "GAME-RAZER-WOLVERINE", price: 1299000, stock: 15, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "GAME-RAZER-WOLVERINE", price: 1349000, stock: 10, rating: 4.4},

		{tenantSlug: "techstore", mpSKU: "GAME-RAZER-KISHI", price: 999000, stock: 20, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "GAME-RAZER-KISHI", price: 1049000, stock: 12, rating: 4.3},

		{tenantSlug: "techstore", mpSKU: "GAME-RAZER-KAIRA", price: 1499000, stock: 18, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "GAME-RAZER-KAIRA", price: 1549000, stock: 10, rating: 4.5},

		// ── Smart Home — Yandex ──────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "HOME-YA-STATION-MAX", price: 1999000, stock: 20, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "HOME-YA-STATION-MAX", price: 2099000, stock: 12, rating: 4.6},

		{tenantSlug: "techstore", mpSKU: "HOME-YA-STATION-MINI", price: 999000, stock: 30, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "HOME-YA-STATION-MINI", price: 1049000, stock: 20, rating: 4.5},

		{tenantSlug: "techstore", mpSKU: "HOME-YA-STATION-LITE", price: 599000, stock: 28, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "HOME-YA-STATION-LITE", price: 629000, stock: 18, rating: 4.3},

		{tenantSlug: "techstore", mpSKU: "HOME-YA-SMART-BTN", price: 399000, stock: 25, rating: 4.3},
		{tenantSlug: "sportmaster", mpSKU: "HOME-YA-SMART-BTN", price: 419000, stock: 20, rating: 4.2},

		// ── Smart Home — Google ──────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "HOME-NEST-HUB2", price: 1299000, stock: 18, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "HOME-NEST-HUB2", price: 1349000, stock: 10, rating: 4.5},

		{tenantSlug: "techstore", mpSKU: "HOME-NEST-MINI", price: 499000, stock: 30, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "HOME-NEST-MINI", price: 529000, stock: 22, rating: 4.4},

		{tenantSlug: "techstore", mpSKU: "HOME-CHROMECAST-4K", price: 699000, stock: 25, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "HOME-CHROMECAST-4K", price: 729000, stock: 15, rating: 4.5},

		// ── Smart Home — Amazon ──────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "HOME-ECHO-DOT5", price: 599000, stock: 25, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "HOME-ECHO-DOT5", price: 629000, stock: 18, rating: 4.4},

		{tenantSlug: "techstore", mpSKU: "HOME-ECHO-SHOW8", price: 1299000, stock: 15, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "HOME-ECHO-SHOW8", price: 1349000, stock: 10, rating: 4.5},

		// ── Smart Home — Xiaomi ──────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "HOME-MI-HUB", price: 299000, stock: 30, rating: 4.4},
		{tenantSlug: "sportmaster", mpSKU: "HOME-MI-HUB", price: 319000, stock: 20, rating: 4.3},

		{tenantSlug: "techstore", mpSKU: "HOME-MI-CAM-2K", price: 399000, stock: 28, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "HOME-MI-CAM-2K", price: 419000, stock: 18, rating: 4.4},

		{tenantSlug: "techstore", mpSKU: "HOME-MI-PURIFIER4", price: 1499000, stock: 12, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "HOME-MI-PURIFIER4", price: 1549000, stock: 8, rating: 4.5},

		// ── Smart Home — Philips ─────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "HOME-HUE-STARTER", price: 1199000, stock: 15, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "HOME-HUE-STARTER", price: 1249000, stock: 10, rating: 4.6},

		{tenantSlug: "techstore", mpSKU: "HOME-HUE-LIGHTSTRIP", price: 699000, stock: 22, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "HOME-HUE-LIGHTSTRIP", price: 729000, stock: 15, rating: 4.4},

		{tenantSlug: "techstore", mpSKU: "HOME-HUE-GO", price: 399000, stock: 20, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "HOME-HUE-GO", price: 419000, stock: 12, rating: 4.5},

		// ── TVs — Samsung ────────────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "TV-SAM-NQLED85", price: 29999000, stock: 3, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "TV-SAM-NQLED85", price: 30499000, stock: 3, rating: 4.7},

		{tenantSlug: "techstore", mpSKU: "TV-SAM-NQLED65", price: 14999000, stock: 8, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "TV-SAM-NQLED65", price: 15499000, stock: 5, rating: 4.7},

		{tenantSlug: "techstore", mpSKU: "TV-SAM-CRYSTAL55", price: 5999000, stock: 15, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "TV-SAM-CRYSTAL55", price: 6199000, stock: 10, rating: 4.4},

		{tenantSlug: "techstore", mpSKU: "TV-SAM-CRYSTAL43", price: 3999000, stock: 20, rating: 4.4},
		{tenantSlug: "sportmaster", mpSKU: "TV-SAM-CRYSTAL43", price: 4199000, stock: 12, rating: 4.3},

		// ── TVs — LG ─────────────────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "TV-LG-OLEDC3-65", price: 24999000, stock: 5, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "TV-LG-OLEDC3-65", price: 25499000, stock: 3, rating: 4.7},

		{tenantSlug: "techstore", mpSKU: "TV-LG-OLEDB3-55", price: 12999000, stock: 8, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "TV-LG-OLEDB3-55", price: 13499000, stock: 5, rating: 4.6},

		{tenantSlug: "techstore", mpSKU: "TV-LG-NANO55", price: 5499000, stock: 15, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "TV-LG-NANO55", price: 5699000, stock: 10, rating: 4.4},

		{tenantSlug: "techstore", mpSKU: "TV-LG-UHD43", price: 3499000, stock: 20, rating: 4.3},
		{tenantSlug: "sportmaster", mpSKU: "TV-LG-UHD43", price: 3699000, stock: 14, rating: 4.2},

		// ── TVs — Sony ───────────────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "TV-SONY-A80L-65", price: 19999000, stock: 5, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "TV-SONY-A80L-65", price: 20499000, stock: 3, rating: 4.7},

		{tenantSlug: "techstore", mpSKU: "TV-SONY-X90L-55", price: 9999000, stock: 10, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "TV-SONY-X90L-55", price: 10499000, stock: 6, rating: 4.6},

		{tenantSlug: "techstore", mpSKU: "TV-SONY-X75L-50", price: 4999000, stock: 15, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "TV-SONY-X75L-50", price: 5199000, stock: 10, rating: 4.4},

		// ── TVs — TCL ────────────────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "TV-TCL-C845-65", price: 7999000, stock: 12, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "TV-TCL-C845-65", price: 8299000, stock: 8, rating: 4.5},

		{tenantSlug: "techstore", mpSKU: "TV-TCL-P745-55", price: 2999000, stock: 20, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "TV-TCL-P745-55", price: 3199000, stock: 14, rating: 4.3},

		// ── TVs — Hisense ────────────────────────────────────────────
		{tenantSlug: "techstore", mpSKU: "TV-HIS-U8K-65", price: 8999000, stock: 10, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "TV-HIS-U8K-65", price: 9299000, stock: 6, rating: 4.6},

		{tenantSlug: "techstore", mpSKU: "TV-HIS-A6K-55", price: 2499000, stock: 22, rating: 4.3},
		{tenantSlug: "sportmaster", mpSKU: "TV-HIS-A6K-55", price: 2699000, stock: 15, rating: 4.2},
	}
}
