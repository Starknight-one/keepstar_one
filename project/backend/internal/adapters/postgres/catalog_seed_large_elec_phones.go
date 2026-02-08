package postgres

func seedElecPhoneProducts() []mp {
	return []mp{
		// ========== SMARTPHONES (30) ==========
		// Apple (6)
		{
			sku: "APL-IP15", name: "iPhone 15", brand: "Apple", catSlug: "smartphones",
			desc:  "The latest iPhone 15 features a 48MP main camera and Dynamic Island for a new way to interact with your phone.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"black","storage":"128GB","display":"6.1 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "APL-IP15P", name: "iPhone 15 Pro", brand: "Apple", catSlug: "smartphones",
			desc:  "iPhone 15 Pro with titanium design, A17 Pro chip, and customizable Action button for pro-level performance.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"natural titanium","storage":"256GB","display":"6.1 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "APL-IP15PM", name: "iPhone 15 Pro Max", brand: "Apple", catSlug: "smartphones",
			desc:  "The biggest iPhone ever with a 6.7-inch Super Retina XDR display, 5x optical zoom, and all-day battery life.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"blue titanium","storage":"512GB","display":"6.7 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "APL-IP14", name: "iPhone 14", brand: "Apple", catSlug: "smartphones",
			desc:  "iPhone 14 delivers an advanced dual-camera system, Crash Detection, and Emergency SOS via satellite.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"midnight","storage":"128GB","display":"6.1 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "APL-IPSE3", name: "iPhone SE 3", brand: "Apple", catSlug: "smartphones",
			desc:  "The most affordable iPhone with the A15 Bionic chip, 5G connectivity, and the classic Touch ID design.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"starlight","storage":"64GB","display":"4.7 inch","type":"budget"}`,
			ownerSlug: "",
		},
		{
			sku: "APL-IP13", name: "iPhone 13", brand: "Apple", catSlug: "smartphones",
			desc:  "iPhone 13 offers a powerful A15 Bionic chip, improved dual-camera system, and a stunning OLED display.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"pink","storage":"128GB","display":"6.1 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		// Samsung (6)
		{
			sku: "SAM-S24U", name: "Galaxy S24 Ultra", brand: "Samsung", catSlug: "smartphones",
			desc:  "Samsung Galaxy S24 Ultra with built-in S Pen, titanium frame, and Galaxy AI for real-time translation and search.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"titanium gray","storage":"256GB","display":"6.8 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "SAM-S24", name: "Galaxy S24", brand: "Samsung", catSlug: "smartphones",
			desc:  "Galaxy S24 brings Galaxy AI features, a vibrant 6.2-inch display, and triple-lens camera in a compact body.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"onyx black","storage":"128GB","display":"6.2 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "SAM-S23FE", name: "Galaxy S23 FE", brand: "Samsung", catSlug: "smartphones",
			desc:  "Galaxy S23 FE delivers flagship features at a more accessible price with a 50MP camera and Snapdragon processor.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"mint","storage":"128GB","display":"6.4 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		{
			sku: "SAM-A54", name: "Galaxy A54", brand: "Samsung", catSlug: "smartphones",
			desc:  "Galaxy A54 5G features a smooth 120Hz Super AMOLED display and water-resistant design for everyday reliability.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"awesome lime","storage":"128GB","display":"6.4 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		{
			sku: "SAM-A34", name: "Galaxy A34", brand: "Samsung", catSlug: "smartphones",
			desc:  "A budget-friendly Samsung phone with a 48MP triple camera, long-lasting battery, and vibrant AMOLED screen.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"awesome silver","storage":"128GB","display":"6.6 inch","type":"budget"}`,
			ownerSlug: "",
		},
		{
			sku: "SAM-ZF5", name: "Galaxy Z Flip 5", brand: "Samsung", catSlug: "smartphones",
			desc:  "The Galaxy Z Flip 5 foldable phone with a large Flex Window, compact clamshell design, and hands-free FlexCam.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"lavender","storage":"256GB","display":"6.7 inch","type":"foldable"}`,
			ownerSlug: "",
		},
		// Google (3)
		{
			sku: "GGL-PX8P", name: "Pixel 8 Pro", brand: "Google", catSlug: "smartphones",
			desc:  "Google Pixel 8 Pro powered by Tensor G3 with advanced AI photo editing, 50MP camera, and seven years of updates.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"bay","storage":"128GB","display":"6.7 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "GGL-PX8", name: "Pixel 8", brand: "Google", catSlug: "smartphones",
			desc:  "Pixel 8 delivers the purest Android experience with AI-powered Best Take, Magic Eraser, and a brilliant Actua display.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"obsidian","storage":"128GB","display":"6.2 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "GGL-PX7A", name: "Pixel 7a", brand: "Google", catSlug: "smartphones",
			desc:  "An affordable Pixel with flagship-level camera, Tensor G2 chip, and wireless charging support.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"charcoal","storage":"128GB","display":"6.1 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		// Xiaomi (4)
		{
			sku: "XMI-14", name: "Xiaomi 14", brand: "Xiaomi", catSlug: "smartphones",
			desc:  "Xiaomi 14 features Leica optics, Snapdragon 8 Gen 3, and a stunning flat AMOLED display with 120Hz refresh.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"black","storage":"256GB","display":"6.36 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "XMI-RN13", name: "Redmi Note 13", brand: "Xiaomi", catSlug: "smartphones",
			desc:  "Redmi Note 13 offers a 108MP camera, AMOLED display, and all-day battery at an unbeatable price point.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"midnight black","storage":"128GB","display":"6.67 inch","type":"budget"}`,
			ownerSlug: "",
		},
		{
			sku: "XMI-POF5", name: "POCO F5", brand: "Xiaomi", catSlug: "smartphones",
			desc:  "POCO F5 packs Snapdragon 7+ Gen 2 performance with 120Hz AMOLED display and 67W turbo charging.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"carbon black","storage":"256GB","display":"6.67 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		{
			sku: "XMI-13T", name: "Xiaomi 13T", brand: "Xiaomi", catSlug: "smartphones",
			desc:  "Xiaomi 13T combines Leica-tuned cameras with a MediaTek Dimensity 8200 for exceptional photography on a budget.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"alpine blue","storage":"256GB","display":"6.67 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		// OnePlus (3)
		{
			sku: "OP-12", name: "OnePlus 12", brand: "OnePlus", catSlug: "smartphones",
			desc:  "OnePlus 12 flagship with Snapdragon 8 Gen 3, Hasselblad camera system, and 100W SUPERVOOC charging.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"flowy emerald","storage":"256GB","display":"6.82 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "OP-NCE3", name: "OnePlus Nord CE 3", brand: "OnePlus", catSlug: "smartphones",
			desc:  "OnePlus Nord CE 3 delivers solid midrange performance with 80W fast charging and a 50MP main camera.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"aqua surge","storage":"128GB","display":"6.7 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		{
			sku: "OP-11", name: "OnePlus 11", brand: "OnePlus", catSlug: "smartphones",
			desc:  "OnePlus 11 with Snapdragon 8 Gen 2, Hasselblad cameras, and alert slider for quick profile switching.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"eternal green","storage":"256GB","display":"6.7 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		// Huawei (3)
		{
			sku: "HW-P60", name: "Huawei P60", brand: "Huawei", catSlug: "smartphones",
			desc:  "Huawei P60 features a stunning XMAGE camera system with variable aperture and an elegant curved display.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"black","storage":"256GB","display":"6.67 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "HW-M60", name: "Mate 60", brand: "Huawei", catSlug: "smartphones",
			desc:  "Huawei Mate 60 delivers a premium experience with satellite calling capability and a powerful Kirin processor.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"dark green","storage":"256GB","display":"6.69 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "HW-NV11", name: "Nova 11", brand: "Huawei", catSlug: "smartphones",
			desc:  "Huawei Nova 11 is a stylish midrange phone with a 60MP front camera designed for outstanding selfies.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"green","storage":"128GB","display":"6.7 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		// Nothing (2)
		{
			sku: "NTH-PH2", name: "Nothing Phone (2)", brand: "Nothing", catSlug: "smartphones",
			desc:  "Nothing Phone (2) features the iconic Glyph Interface, Snapdragon 8+ Gen 1, and a clean Nothing OS experience.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"white","storage":"256GB","display":"6.7 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "NTH-PH1", name: "Nothing Phone (1)", brand: "Nothing", catSlug: "smartphones",
			desc:  "The original Nothing Phone with transparent Glyph lights, Snapdragon 778G+, and a distinctly minimalist design.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"black","storage":"128GB","display":"6.55 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		// Realme (3)
		{
			sku: "RLM-GT5", name: "Realme GT 5", brand: "Realme", catSlug: "smartphones",
			desc:  "Realme GT 5 packs Snapdragon 8 Gen 2 power with 240W fast charging for a full charge in under ten minutes.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"purple","storage":"256GB","display":"6.74 inch","type":"flagship"}`,
			ownerSlug: "",
		},
		{
			sku: "RLM-11P", name: "Realme 11 Pro", brand: "Realme", catSlug: "smartphones",
			desc:  "Realme 11 Pro features a 100MP OIS camera with a premium vegan leather back for a unique look and feel.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"sunrise beige","storage":"128GB","display":"6.7 inch","type":"midrange"}`,
			ownerSlug: "",
		},
		{
			sku: "RLM-NZ60", name: "Narzo 60", brand: "Realme", catSlug: "smartphones",
			desc:  "Realme Narzo 60 offers a 90Hz AMOLED display and 33W fast charging at an entry-level price point.",
			images: `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800"]`,
			attrs:  `{"color":"mars orange","storage":"128GB","display":"6.4 inch","type":"budget"}`,
			ownerSlug: "",
		},

		// ========== TABLETS (15) ==========
		// Apple (5)
		{
			sku: "TAB-IPADP129", name: "iPad Pro 12.9", brand: "Apple", catSlug: "tablets",
			desc:  "iPad Pro 12.9-inch with M2 chip, Liquid Retina XDR display, and Thunderbolt connectivity for professional workflows.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"space gray","storage":"256GB","display":"12.9 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-IPADP11", name: "iPad Pro 11", brand: "Apple", catSlug: "tablets",
			desc:  "iPad Pro 11-inch delivers M2 performance in a portable form factor with ProMotion and Face ID.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"silver","storage":"128GB","display":"11 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-IPADA", name: "iPad Air", brand: "Apple", catSlug: "tablets",
			desc:  "iPad Air with M1 chip offers a perfect balance of performance and portability with a 10.9-inch Liquid Retina display.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"blue","storage":"64GB","display":"10.9 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-IPAD10", name: "iPad 10th Gen", brand: "Apple", catSlug: "tablets",
			desc:  "The redesigned iPad 10th generation features an A14 Bionic chip, USB-C, and a vibrant 10.9-inch Liquid Retina display.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"yellow","storage":"64GB","display":"10.9 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-IPADM", name: "iPad mini", brand: "Apple", catSlug: "tablets",
			desc:  "iPad mini packs the A15 Bionic chip into an ultra-portable 8.3-inch form factor with Apple Pencil support.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"starlight","storage":"64GB","display":"8.3 inch"}`,
			ownerSlug: "",
		},
		// Samsung (4)
		{
			sku: "TAB-S9U", name: "Galaxy Tab S9 Ultra", brand: "Samsung", catSlug: "tablets",
			desc:  "Samsung Galaxy Tab S9 Ultra features a massive 14.6-inch Dynamic AMOLED 2X display and Snapdragon 8 Gen 2 for Galaxy.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"graphite","storage":"256GB","display":"14.6 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-S9", name: "Galaxy Tab S9", brand: "Samsung", catSlug: "tablets",
			desc:  "Galaxy Tab S9 delivers premium performance with a Dynamic AMOLED 2X display and IP68 water resistance.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"beige","storage":"128GB","display":"11 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-A9", name: "Galaxy Tab A9", brand: "Samsung", catSlug: "tablets",
			desc:  "Galaxy Tab A9 is an affordable everyday tablet with a smooth display and long-lasting battery for entertainment.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"navy","storage":"64GB","display":"8.7 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-S9FE", name: "Galaxy Tab S9 FE", brand: "Samsung", catSlug: "tablets",
			desc:  "Galaxy Tab S9 FE brings S Pen creativity and a vivid display at a fan-edition price point.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"mint","storage":"128GB","display":"10.9 inch"}`,
			ownerSlug: "",
		},
		// Lenovo (3)
		{
			sku: "TAB-LP12P", name: "Lenovo Tab P12 Pro", brand: "Lenovo", catSlug: "tablets",
			desc:  "Lenovo Tab P12 Pro features a 12.6-inch AMOLED display with JBL quad speakers for immersive media consumption.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"storm grey","storage":"256GB","display":"12.6 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-LP11", name: "Lenovo Tab P11", brand: "Lenovo", catSlug: "tablets",
			desc:  "Lenovo Tab P11 offers a balanced Android tablet experience with Dolby Atmos sound and a crisp 2K display.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"slate grey","storage":"128GB","display":"11.5 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-LM10", name: "Lenovo Tab M10", brand: "Lenovo", catSlug: "tablets",
			desc:  "Lenovo Tab M10 is a family-friendly tablet with parental controls, dual speakers, and a 10.1-inch display.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"iron grey","storage":"64GB","display":"10.1 inch"}`,
			ownerSlug: "",
		},
		// Huawei (3)
		{
			sku: "TAB-HMPP", name: "Huawei MatePad Pro", brand: "Huawei", catSlug: "tablets",
			desc:  "Huawei MatePad Pro features an OLED FullView display and M-Pencil support for creative professionals.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"golden black","storage":"256GB","display":"11 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-HMP11", name: "Huawei MatePad 11", brand: "Huawei", catSlug: "tablets",
			desc:  "Huawei MatePad 11 offers a 120Hz display with Harman Kardon speakers for an immersive entertainment experience.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"matte grey","storage":"128GB","display":"11 inch"}`,
			ownerSlug: "",
		},
		{
			sku: "TAB-HMPSE", name: "Huawei MatePad SE", brand: "Huawei", catSlug: "tablets",
			desc:  "Huawei MatePad SE is an entry-level tablet with eye-comfort display and a lightweight design for everyday use.",
			images: `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`,
			attrs:  `{"color":"graphite black","storage":"64GB","display":"10.4 inch"}`,
			ownerSlug: "",
		},

		// ========== SMARTWATCHES (20) ==========
		// Apple (4)
		{
			sku: "WATCH-AWU2", name: "Apple Watch Ultra 2", brand: "Apple", catSlug: "smartwatches",
			desc:  "Apple Watch Ultra 2 is built for extreme adventures with a 49mm titanium case, precision dual-frequency GPS, and 36-hour battery.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"natural titanium","size":"49mm","type":"sport"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-AWS9", name: "Apple Watch Series 9", brand: "Apple", catSlug: "smartwatches",
			desc:  "Apple Watch Series 9 with S9 chip enables Double Tap gesture, brighter display, and on-device Siri.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"midnight","size":"45mm","type":"classic"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-AWSE2", name: "Apple Watch SE 2", brand: "Apple", catSlug: "smartwatches",
			desc:  "Apple Watch SE 2 delivers essential smartwatch features including Crash Detection at an accessible price.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"starlight","size":"44mm","type":"fitness"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-AWS8", name: "Apple Watch Series 8", brand: "Apple", catSlug: "smartwatches",
			desc:  "Apple Watch Series 8 features advanced health sensors including temperature sensing and blood oxygen monitoring.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"product red","size":"45mm","type":"classic"}`,
			ownerSlug: "",
		},
		// Samsung (3)
		{
			sku: "WATCH-GW6C", name: "Galaxy Watch 6 Classic", brand: "Samsung", catSlug: "smartwatches",
			desc:  "Galaxy Watch 6 Classic brings back the rotating bezel with a premium stainless steel design and advanced health tracking.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"silver","size":"47mm","type":"classic"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-GW6", name: "Galaxy Watch 6", brand: "Samsung", catSlug: "smartwatches",
			desc:  "Galaxy Watch 6 features a slimmer design, larger display, and enhanced sleep coaching powered by Samsung Health.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"graphite","size":"44mm","type":"fitness"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-GWFE", name: "Galaxy Watch FE", brand: "Samsung", catSlug: "smartwatches",
			desc:  "Galaxy Watch FE offers essential Samsung smartwatch features with BIA body composition and sleep tracking at a lower price.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"black","size":"40mm","type":"fitness"}`,
			ownerSlug: "",
		},
		// Garmin (4)
		{
			sku: "WATCH-GF7", name: "Garmin Fenix 7", brand: "Garmin", catSlug: "smartwatches",
			desc:  "Garmin Fenix 7 is the ultimate multisport GPS watch with solar charging, topo maps, and up to 22 days battery life.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"slate gray","size":"47mm","type":"sport"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-GV3", name: "Garmin Venu 3", brand: "Garmin", catSlug: "smartwatches",
			desc:  "Garmin Venu 3 combines a vivid AMOLED display with advanced health monitoring and up to 14 days of battery life.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"black","size":"45mm","type":"classic"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-GFR265", name: "Garmin Forerunner 265", brand: "Garmin", catSlug: "smartwatches",
			desc:  "Garmin Forerunner 265 is a running-focused GPS watch with AMOLED display and training readiness insights.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"whitestone","size":"46mm","type":"sport"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-GI2", name: "Garmin Instinct 2", brand: "Garmin", catSlug: "smartwatches",
			desc:  "Garmin Instinct 2 is a rugged outdoor GPS watch built to military standards with unlimited solar battery life.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"graphite","size":"45mm","type":"sport"}`,
			ownerSlug: "",
		},
		// Fitbit (3)
		{
			sku: "WATCH-FS2", name: "Fitbit Sense 2", brand: "Fitbit", catSlug: "smartwatches",
			desc:  "Fitbit Sense 2 tracks stress, heart health, and sleep with a continuous EDA sensor and built-in GPS.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"shadow grey","size":"40mm","type":"fitness"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-FV4", name: "Fitbit Versa 4", brand: "Fitbit", catSlug: "smartwatches",
			desc:  "Fitbit Versa 4 is a fitness-focused smartwatch with 40+ exercise modes, built-in GPS, and six-day battery life.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"beet juice","size":"40mm","type":"fitness"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-FC6", name: "Fitbit Charge 6", brand: "Fitbit", catSlug: "smartwatches",
			desc:  "Fitbit Charge 6 is a compact fitness tracker with Google integration, heart rate tracking, and seven-day battery.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"obsidian","size":"one size","type":"fitness"}`,
			ownerSlug: "",
		},
		// Huawei (3)
		{
			sku: "WATCH-HGT4", name: "Huawei Watch GT 4", brand: "Huawei", catSlug: "smartwatches",
			desc:  "Huawei Watch GT 4 features an elegant octagonal design with up to 14 days battery and TruSeen heart rate monitoring.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"brown leather","size":"46mm","type":"classic"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-HW4P", name: "Huawei Watch 4 Pro", brand: "Huawei", catSlug: "smartwatches",
			desc:  "Huawei Watch 4 Pro offers eSIM calling, sapphire glass, and professional health management with ECG analysis.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"dark brown","size":"48mm","type":"classic"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-HB8", name: "Huawei Band 8", brand: "Huawei", catSlug: "smartwatches",
			desc:  "Huawei Band 8 is a slim and lightweight fitness band with AMOLED display and comprehensive sleep tracking.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"midnight black","size":"one size","type":"fitness"}`,
			ownerSlug: "",
		},
		// Xiaomi (3)
		{
			sku: "WATCH-XWS3", name: "Xiaomi Watch S3", brand: "Xiaomi", catSlug: "smartwatches",
			desc:  "Xiaomi Watch S3 features a swappable bezel design, AMOLED display, and dual-band GPS for outdoor activities.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"black","size":"47mm","type":"sport"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-XW2", name: "Xiaomi Watch 2", brand: "Xiaomi", catSlug: "smartwatches",
			desc:  "Xiaomi Watch 2 runs Wear OS with Google apps support, offering a full smartwatch experience at an affordable price.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"silver","size":"46mm","type":"classic"}`,
			ownerSlug: "",
		},
		{
			sku: "WATCH-XSB8", name: "Xiaomi Smart Band 8", brand: "Xiaomi", catSlug: "smartwatches",
			desc:  "Xiaomi Smart Band 8 is a versatile fitness tracker with a vibrant AMOLED display and 16-day battery life.",
			images: `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`,
			attrs:  `{"color":"black","size":"one size","type":"fitness"}`,
			ownerSlug: "",
		},
	}
}

func seedElecPhoneListings() []listing {
	return []listing{
		// ========== SMARTPHONES ==========
		// Apple iPhone 15
		{tenantSlug: "techstore", mpSKU: "APL-IP15", price: 8999000, stock: 30, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "APL-IP15", price: 9199000, stock: 15, rating: 4.6},
		// Apple iPhone 15 Pro
		{tenantSlug: "techstore", mpSKU: "APL-IP15P", price: 12999000, stock: 25, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "APL-IP15P", price: 13299000, stock: 10, rating: 4.7},
		// Apple iPhone 15 Pro Max
		{tenantSlug: "techstore", mpSKU: "APL-IP15PM", price: 17999000, stock: 20, rating: 4.9},
		{tenantSlug: "fashionhub", mpSKU: "APL-IP15PM", price: 17999000, stock: 8, rating: 4.8},
		// Apple iPhone 14
		{tenantSlug: "techstore", mpSKU: "APL-IP14", price: 7999000, stock: 35, rating: 4.5},
		{tenantSlug: "nike", mpSKU: "APL-IP14", price: 8199000, stock: 12, rating: 4.4},
		// Apple iPhone SE 3
		{tenantSlug: "techstore", mpSKU: "APL-IPSE3", price: 6999000, stock: 40, rating: 4.2},
		{tenantSlug: "nike", mpSKU: "APL-IPSE3", price: 7199000, stock: 18, rating: 4.1},
		// Apple iPhone 13
		{tenantSlug: "techstore", mpSKU: "APL-IP13", price: 6999000, stock: 30, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "APL-IP13", price: 7299000, stock: 14, rating: 4.3},
		// Samsung Galaxy S24 Ultra
		{tenantSlug: "techstore", mpSKU: "SAM-S24U", price: 14999000, stock: 25, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "SAM-S24U", price: 15299000, stock: 10, rating: 4.7},
		// Samsung Galaxy S24
		{tenantSlug: "techstore", mpSKU: "SAM-S24", price: 8999000, stock: 35, rating: 4.6},
		{tenantSlug: "nike", mpSKU: "SAM-S24", price: 9199000, stock: 15, rating: 4.5},
		// Samsung Galaxy S23 FE
		{tenantSlug: "techstore", mpSKU: "SAM-S23FE", price: 5999000, stock: 30, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "SAM-S23FE", price: 6199000, stock: 20, rating: 4.3},
		// Samsung Galaxy A54
		{tenantSlug: "techstore", mpSKU: "SAM-A54", price: 3999000, stock: 45, rating: 4.3},
		{tenantSlug: "nike", mpSKU: "SAM-A54", price: 4199000, stock: 25, rating: 4.2},
		// Samsung Galaxy A34
		{tenantSlug: "techstore", mpSKU: "SAM-A34", price: 2499000, stock: 50, rating: 4.1},
		{tenantSlug: "fashionhub", mpSKU: "SAM-A34", price: 2699000, stock: 30, rating: 4.0},
		// Samsung Galaxy Z Flip 5
		{tenantSlug: "techstore", mpSKU: "SAM-ZF5", price: 10999000, stock: 15, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "SAM-ZF5", price: 11299000, stock: 8, rating: 4.4},
		// Google Pixel 8 Pro
		{tenantSlug: "techstore", mpSKU: "GGL-PX8P", price: 8499000, stock: 20, rating: 4.7},
		{tenantSlug: "nike", mpSKU: "GGL-PX8P", price: 8699000, stock: 10, rating: 4.6},
		// Google Pixel 8
		{tenantSlug: "techstore", mpSKU: "GGL-PX8", price: 6499000, stock: 25, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "GGL-PX8", price: 6699000, stock: 12, rating: 4.5},
		// Google Pixel 7a
		{tenantSlug: "techstore", mpSKU: "GGL-PX7A", price: 3999000, stock: 30, rating: 4.4},
		{tenantSlug: "nike", mpSKU: "GGL-PX7A", price: 4199000, stock: 18, rating: 4.3},
		// Xiaomi 14
		{tenantSlug: "techstore", mpSKU: "XMI-14", price: 5999000, stock: 25, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "XMI-14", price: 6199000, stock: 15, rating: 4.4},
		// Redmi Note 13
		{tenantSlug: "techstore", mpSKU: "XMI-RN13", price: 1999000, stock: 50, rating: 4.2},
		{tenantSlug: "nike", mpSKU: "XMI-RN13", price: 2099000, stock: 30, rating: 4.1},
		// POCO F5
		{tenantSlug: "techstore", mpSKU: "XMI-POF5", price: 3499000, stock: 35, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "XMI-POF5", price: 3599000, stock: 20, rating: 4.3},
		// Xiaomi 13T
		{tenantSlug: "techstore", mpSKU: "XMI-13T", price: 3999000, stock: 30, rating: 4.3},
		{tenantSlug: "nike", mpSKU: "XMI-13T", price: 4199000, stock: 18, rating: 4.2},
		// OnePlus 12
		{tenantSlug: "techstore", mpSKU: "OP-12", price: 6499000, stock: 20, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "OP-12", price: 6699000, stock: 12, rating: 4.5},
		// OnePlus Nord CE 3
		{tenantSlug: "techstore", mpSKU: "OP-NCE3", price: 2999000, stock: 35, rating: 4.2},
		{tenantSlug: "nike", mpSKU: "OP-NCE3", price: 3199000, stock: 20, rating: 4.1},
		// OnePlus 11
		{tenantSlug: "techstore", mpSKU: "OP-11", price: 5499000, stock: 20, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "OP-11", price: 5699000, stock: 10, rating: 4.4},
		// Huawei P60
		{tenantSlug: "techstore", mpSKU: "HW-P60", price: 5499000, stock: 15, rating: 4.3},
		{tenantSlug: "nike", mpSKU: "HW-P60", price: 5699000, stock: 8, rating: 4.2},
		// Huawei Mate 60
		{tenantSlug: "techstore", mpSKU: "HW-M60", price: 7999000, stock: 10, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "HW-M60", price: 8199000, stock: 5, rating: 4.4},
		// Huawei Nova 11
		{tenantSlug: "techstore", mpSKU: "HW-NV11", price: 2999000, stock: 25, rating: 4.1},
		{tenantSlug: "nike", mpSKU: "HW-NV11", price: 3199000, stock: 15, rating: 4.0},
		// Nothing Phone (2)
		{tenantSlug: "techstore", mpSKU: "NTH-PH2", price: 4999000, stock: 20, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "NTH-PH2", price: 5199000, stock: 10, rating: 4.3},
		// Nothing Phone (1)
		{tenantSlug: "techstore", mpSKU: "NTH-PH1", price: 2999000, stock: 15, rating: 4.2},
		{tenantSlug: "nike", mpSKU: "NTH-PH1", price: 3199000, stock: 8, rating: 4.1},
		// Realme GT 5
		{tenantSlug: "techstore", mpSKU: "RLM-GT5", price: 4499000, stock: 20, rating: 4.3},
		{tenantSlug: "fashionhub", mpSKU: "RLM-GT5", price: 4699000, stock: 12, rating: 4.2},
		// Realme 11 Pro
		{tenantSlug: "techstore", mpSKU: "RLM-11P", price: 2999000, stock: 30, rating: 4.2},
		{tenantSlug: "nike", mpSKU: "RLM-11P", price: 3199000, stock: 18, rating: 4.1},
		// Narzo 60
		{tenantSlug: "techstore", mpSKU: "RLM-NZ60", price: 1999000, stock: 40, rating: 4.0},
		{tenantSlug: "fashionhub", mpSKU: "RLM-NZ60", price: 2099000, stock: 25, rating: 4.0},

		// ========== TABLETS ==========
		// iPad Pro 12.9
		{tenantSlug: "techstore", mpSKU: "TAB-IPADP129", price: 12999000, stock: 15, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "TAB-IPADP129", price: 13499000, stock: 8, rating: 4.7},
		// iPad Pro 11
		{tenantSlug: "techstore", mpSKU: "TAB-IPADP11", price: 8999000, stock: 20, rating: 4.7},
		{tenantSlug: "nike", mpSKU: "TAB-IPADP11", price: 9299000, stock: 10, rating: 4.6},
		// iPad Air
		{tenantSlug: "techstore", mpSKU: "TAB-IPADA", price: 6999000, stock: 25, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "TAB-IPADA", price: 7299000, stock: 12, rating: 4.5},
		// iPad 10th Gen
		{tenantSlug: "techstore", mpSKU: "TAB-IPAD10", price: 4999000, stock: 35, rating: 4.4},
		{tenantSlug: "nike", mpSKU: "TAB-IPAD10", price: 5199000, stock: 20, rating: 4.3},
		// iPad mini
		{tenantSlug: "techstore", mpSKU: "TAB-IPADM", price: 6499000, stock: 20, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "TAB-IPADM", price: 6799000, stock: 10, rating: 4.4},
		// Galaxy Tab S9 Ultra
		{tenantSlug: "techstore", mpSKU: "TAB-S9U", price: 11999000, stock: 10, rating: 4.7},
		{tenantSlug: "nike", mpSKU: "TAB-S9U", price: 12499000, stock: 5, rating: 4.6},
		// Galaxy Tab S9
		{tenantSlug: "techstore", mpSKU: "TAB-S9", price: 7999000, stock: 20, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "TAB-S9", price: 8299000, stock: 12, rating: 4.4},
		// Galaxy Tab A9
		{tenantSlug: "techstore", mpSKU: "TAB-A9", price: 1999000, stock: 40, rating: 4.1},
		{tenantSlug: "nike", mpSKU: "TAB-A9", price: 2099000, stock: 25, rating: 4.0},
		// Galaxy Tab S9 FE
		{tenantSlug: "techstore", mpSKU: "TAB-S9FE", price: 4499000, stock: 25, rating: 4.3},
		{tenantSlug: "fashionhub", mpSKU: "TAB-S9FE", price: 4699000, stock: 15, rating: 4.2},
		// Lenovo Tab P12 Pro
		{tenantSlug: "techstore", mpSKU: "TAB-LP12P", price: 5999000, stock: 15, rating: 4.4},
		{tenantSlug: "nike", mpSKU: "TAB-LP12P", price: 6199000, stock: 8, rating: 4.3},
		// Lenovo Tab P11
		{tenantSlug: "techstore", mpSKU: "TAB-LP11", price: 2999000, stock: 30, rating: 4.2},
		{tenantSlug: "fashionhub", mpSKU: "TAB-LP11", price: 3199000, stock: 18, rating: 4.1},
		// Lenovo Tab M10
		{tenantSlug: "techstore", mpSKU: "TAB-LM10", price: 1499000, stock: 45, rating: 4.0},
		{tenantSlug: "nike", mpSKU: "TAB-LM10", price: 1599000, stock: 30, rating: 4.0},
		// Huawei MatePad Pro
		{tenantSlug: "techstore", mpSKU: "TAB-HMPP", price: 5499000, stock: 15, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "TAB-HMPP", price: 5699000, stock: 8, rating: 4.3},
		// Huawei MatePad 11
		{tenantSlug: "techstore", mpSKU: "TAB-HMP11", price: 3499000, stock: 20, rating: 4.2},
		{tenantSlug: "nike", mpSKU: "TAB-HMP11", price: 3699000, stock: 12, rating: 4.1},
		// Huawei MatePad SE
		{tenantSlug: "techstore", mpSKU: "TAB-HMPSE", price: 1999000, stock: 35, rating: 4.0},
		{tenantSlug: "fashionhub", mpSKU: "TAB-HMPSE", price: 2099000, stock: 20, rating: 4.0},

		// ========== SMARTWATCHES ==========
		// Apple Watch Ultra 2
		{tenantSlug: "techstore", mpSKU: "WATCH-AWU2", price: 8999000, stock: 10, rating: 4.9},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-AWU2", price: 9299000, stock: 5, rating: 4.8},
		// Apple Watch Series 9
		{tenantSlug: "techstore", mpSKU: "WATCH-AWS9", price: 4499000, stock: 25, rating: 4.7},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-AWS9", price: 4699000, stock: 12, rating: 4.6},
		// Apple Watch SE 2
		{tenantSlug: "techstore", mpSKU: "WATCH-AWSE2", price: 2799000, stock: 30, rating: 4.4},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-AWSE2", price: 2999000, stock: 18, rating: 4.3},
		// Apple Watch Series 8
		{tenantSlug: "techstore", mpSKU: "WATCH-AWS8", price: 3999000, stock: 20, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "WATCH-AWS8", price: 4199000, stock: 10, rating: 4.4},
		// Galaxy Watch 6 Classic
		{tenantSlug: "techstore", mpSKU: "WATCH-GW6C", price: 3499000, stock: 15, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-GW6C", price: 3699000, stock: 8, rating: 4.4},
		// Galaxy Watch 6
		{tenantSlug: "techstore", mpSKU: "WATCH-GW6", price: 2999000, stock: 25, rating: 4.4},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-GW6", price: 3199000, stock: 15, rating: 4.3},
		// Galaxy Watch FE
		{tenantSlug: "techstore", mpSKU: "WATCH-GWFE", price: 1999000, stock: 30, rating: 4.2},
		{tenantSlug: "fashionhub", mpSKU: "WATCH-GWFE", price: 2099000, stock: 20, rating: 4.1},
		// Garmin Fenix 7
		{tenantSlug: "techstore", mpSKU: "WATCH-GF7", price: 6999000, stock: 10, rating: 4.8},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-GF7", price: 7199000, stock: 5, rating: 4.7},
		// Garmin Venu 3
		{tenantSlug: "techstore", mpSKU: "WATCH-GV3", price: 4499000, stock: 15, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-GV3", price: 4699000, stock: 8, rating: 4.5},
		// Garmin Forerunner 265
		{tenantSlug: "techstore", mpSKU: "WATCH-GFR265", price: 3999000, stock: 20, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-GFR265", price: 4199000, stock: 10, rating: 4.5},
		// Garmin Instinct 2
		{tenantSlug: "techstore", mpSKU: "WATCH-GI2", price: 2999000, stock: 20, rating: 4.5},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-GI2", price: 3199000, stock: 12, rating: 4.4},
		// Fitbit Sense 2
		{tenantSlug: "techstore", mpSKU: "WATCH-FS2", price: 2499000, stock: 20, rating: 4.3},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-FS2", price: 2699000, stock: 12, rating: 4.2},
		// Fitbit Versa 4
		{tenantSlug: "techstore", mpSKU: "WATCH-FV4", price: 1999000, stock: 25, rating: 4.2},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-FV4", price: 2199000, stock: 15, rating: 4.1},
		// Fitbit Charge 6
		{tenantSlug: "techstore", mpSKU: "WATCH-FC6", price: 1499000, stock: 35, rating: 4.3},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-FC6", price: 1599000, stock: 20, rating: 4.2},
		// Huawei Watch GT 4
		{tenantSlug: "techstore", mpSKU: "WATCH-HGT4", price: 2499000, stock: 20, rating: 4.4},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-HGT4", price: 2699000, stock: 10, rating: 4.3},
		// Huawei Watch 4 Pro
		{tenantSlug: "techstore", mpSKU: "WATCH-HW4P", price: 4499000, stock: 10, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "WATCH-HW4P", price: 4699000, stock: 5, rating: 4.4},
		// Huawei Band 8
		{tenantSlug: "techstore", mpSKU: "WATCH-HB8", price: 399000, stock: 50, rating: 4.1},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-HB8", price: 449000, stock: 30, rating: 4.0},
		// Xiaomi Watch S3
		{tenantSlug: "techstore", mpSKU: "WATCH-XWS3", price: 1299000, stock: 25, rating: 4.3},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-XWS3", price: 1399000, stock: 15, rating: 4.2},
		// Xiaomi Watch 2
		{tenantSlug: "techstore", mpSKU: "WATCH-XW2", price: 1799000, stock: 20, rating: 4.2},
		{tenantSlug: "fashionhub", mpSKU: "WATCH-XW2", price: 1899000, stock: 12, rating: 4.1},
		// Xiaomi Smart Band 8
		{tenantSlug: "techstore", mpSKU: "WATCH-XSB8", price: 299000, stock: 50, rating: 4.2},
		{tenantSlug: "sportmaster", mpSKU: "WATCH-XSB8", price: 349000, stock: 35, rating: 4.1},
	}
}
