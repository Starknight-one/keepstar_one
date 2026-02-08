package postgres

func seedElecAudioProducts() []mp {
	return []mp{
		// ───────────────────────────── Laptops (30) ─────────────────────────────

		// Apple (5)
		{
			sku: "LAP-APPLE-MBP16M3MAX", name: "MacBook Pro 16 M3 Max",
			desc:  "The most powerful MacBook Pro ever, featuring the M3 Max chip with a 16-inch Liquid Retina XDR display for demanding creative workflows.",
			brand: "Apple", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1517336714731-489689fd1ca8?w=600"]`,
			attrs:    `{"color":"Space Black","storage":"1TB SSD","ram":"36GB","display":"16.2 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-APPLE-MBP14M3PRO", name: "MacBook Pro 14 M3 Pro",
			desc:  "Pro-level performance in a portable 14-inch form factor, powered by the M3 Pro chip with up to 18 hours of battery life.",
			brand: "Apple", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1611186871348-b1ce696e52c9?w=600"]`,
			attrs:    `{"color":"Silver","storage":"512GB SSD","ram":"18GB","display":"14.2 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-APPLE-MBA15M3", name: "MacBook Air 15 M3",
			desc:  "The remarkably thin 15-inch MacBook Air with M3 chip delivers outstanding performance and an expansive display for everyday productivity.",
			brand: "Apple", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1541807084-5c52b6b3adef?w=600"]`,
			attrs:    `{"color":"Midnight","storage":"512GB SSD","ram":"16GB","display":"15.3 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-APPLE-MBA13M3", name: "MacBook Air 13 M3",
			desc:  "Strikingly thin and fast, the 13-inch MacBook Air with M3 is the perfect everyday laptop with all-day battery life.",
			brand: "Apple", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1517336714731-489689fd1ca8?w=600"]`,
			attrs:    `{"color":"Starlight","storage":"256GB SSD","ram":"8GB","display":"13.6 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-APPLE-MBP13M2", name: "MacBook Pro 13 M2",
			desc:  "Compact and capable, the 13-inch MacBook Pro with M2 chip offers exceptional performance with the beloved Touch Bar design.",
			brand: "Apple", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1611186871348-b1ce696e52c9?w=600"]`,
			attrs:    `{"color":"Space Gray","storage":"512GB SSD","ram":"16GB","display":"13.3 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},

		// Dell (4)
		{
			sku: "LAP-DELL-XPS15", name: "Dell XPS 15",
			desc:  "Premium ultrabook with a stunning 15.6-inch OLED display, Intel Core i7-13700H, and a sleek aluminium chassis.",
			brand: "Dell", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1593642632559-0c6d3fc62b89?w=600"]`,
			attrs:    `{"color":"Platinum Silver","storage":"512GB SSD","ram":"16GB","display":"15.6 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-DELL-XPS13", name: "Dell XPS 13",
			desc:  "Ultra-portable 13-inch laptop with InfinityEdge display and 12th Gen Intel processor, weighing just 1.17 kg.",
			brand: "Dell", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1593642632559-0c6d3fc62b89?w=600"]`,
			attrs:    `{"color":"Sky Blue","storage":"256GB SSD","ram":"8GB","display":"13.4 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-DELL-INSP16", name: "Dell Inspiron 16",
			desc:  "Versatile 16-inch laptop for work and entertainment with a comfortable keyboard and long-lasting battery.",
			brand: "Dell", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=600"]`,
			attrs:    `{"color":"Dark Green","storage":"512GB SSD","ram":"16GB","display":"16 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-DELL-LAT14", name: "Dell Latitude 14",
			desc:  "Business-class laptop with enterprise security features, Intel vPro, and a durable build for the mobile workforce.",
			brand: "Dell", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=600"]`,
			attrs:    `{"color":"Black","storage":"256GB SSD","ram":"16GB","display":"14 inch","type":"business"}`,
			ownerSlug: "",
		},

		// Lenovo (5)
		{
			sku: "LAP-LEN-X1CARBON", name: "Lenovo ThinkPad X1 Carbon Gen 11",
			desc:  "Legendary business ultrabook with a 14-inch 2.8K OLED display, Intel Core i7-1365U, and MIL-STD-810H durability.",
			brand: "Lenovo", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=600"]`,
			attrs:    `{"color":"Black","storage":"512GB SSD","ram":"16GB","display":"14 inch","type":"business"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-LEN-T14", name: "Lenovo ThinkPad T14 Gen 4",
			desc:  "Reliable business workhorse with AMD Ryzen 7 PRO processor and a bright 14-inch display for on-the-go professionals.",
			brand: "Lenovo", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=600"]`,
			attrs:    `{"color":"Storm Grey","storage":"512GB SSD","ram":"16GB","display":"14 inch","type":"business"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-LEN-IDEAPAD5", name: "Lenovo IdeaPad 5",
			desc:  "Affordable yet capable 15.6-inch laptop with AMD Ryzen 5, ideal for students and everyday computing tasks.",
			brand: "Lenovo", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1496181133206-80ce9b88a853?w=600"]`,
			attrs:    `{"color":"Cloud Grey","storage":"256GB SSD","ram":"8GB","display":"15.6 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-LEN-LEGPRO5", name: "Lenovo Legion Pro 5",
			desc:  "High-performance gaming laptop with RTX 4070, 165Hz WQHD display, and advanced thermal management for sustained performance.",
			brand: "Lenovo", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1603302576837-37561b2e2302?w=600"]`,
			attrs:    `{"color":"Onyx Grey","storage":"1TB SSD","ram":"32GB","display":"16 inch","type":"gaming"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-LEN-YOGA9I", name: "Lenovo Yoga 9i",
			desc:  "Premium 2-in-1 convertible with a 14-inch OLED PureSight display, rotating soundbar, and Intel Evo platform certification.",
			brand: "Lenovo", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1496181133206-80ce9b88a853?w=600"]`,
			attrs:    `{"color":"Oatmeal","storage":"512GB SSD","ram":"16GB","display":"14 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},

		// HP (4)
		{
			sku: "LAP-HP-SPECX360", name: "HP Spectre x360 14",
			desc:  "Stunning 2-in-1 convertible with a 14-inch 3K2K OLED display, gem-cut design, and Intel Core i7 processor.",
			brand: "HP", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1496181133206-80ce9b88a853?w=600"]`,
			attrs:    `{"color":"Nightfall Black","storage":"1TB SSD","ram":"16GB","display":"14 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-HP-PAV15", name: "HP Pavilion 15",
			desc:  "Well-rounded everyday laptop with a 15.6-inch IPS display, Intel Core i5, and a micro-edge bezel design.",
			brand: "HP", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=600"]`,
			attrs:    `{"color":"Fog Blue","storage":"512GB SSD","ram":"8GB","display":"15.6 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-HP-EB840", name: "HP EliteBook 840 G10",
			desc:  "Enterprise-grade business laptop with Intel vPro, Sure View privacy screen, and Wolf Security for maximum protection.",
			brand: "HP", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=600"]`,
			attrs:    `{"color":"Silver","storage":"512GB SSD","ram":"16GB","display":"14 inch","type":"business"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-HP-OMEN16", name: "HP Omen 16",
			desc:  "Powerful gaming laptop with RTX 4060, 165Hz QHD display, and Omen Tempest cooling for peak gaming sessions.",
			brand: "HP", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1603302576837-37561b2e2302?w=600"]`,
			attrs:    `{"color":"Shadow Black","storage":"1TB SSD","ram":"16GB","display":"16.1 inch","type":"gaming"}`,
			ownerSlug: "",
		},

		// Asus (4)
		{
			sku: "LAP-ASUS-ZB14", name: "Asus ZenBook 14 OLED",
			desc:  "Ultra-slim 14-inch laptop with a vivid OLED display, Intel Core i7-1360P, and weighing just 1.39 kg for premium portability.",
			brand: "Asus", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1496181133206-80ce9b88a853?w=600"]`,
			attrs:    `{"color":"Ponder Blue","storage":"512GB SSD","ram":"16GB","display":"14 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-ASUS-ROGG16", name: "Asus ROG Strix G16",
			desc:  "Gaming powerhouse with RTX 4070, 240Hz display, and intelligent cooling that keeps noise levels under control.",
			brand: "Asus", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1603302576837-37561b2e2302?w=600"]`,
			attrs:    `{"color":"Eclipse Gray","storage":"1TB SSD","ram":"16GB","display":"16 inch","type":"gaming"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-ASUS-VB15", name: "Asus VivoBook 15",
			desc:  "Affordable everyday laptop with a 15.6-inch NanoEdge display, ergonomic hinge design, and vibrant colour options.",
			brand: "Asus", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1496181133206-80ce9b88a853?w=600"]`,
			attrs:    `{"color":"Quiet Blue","storage":"256GB SSD","ram":"8GB","display":"15.6 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-ASUS-PROART", name: "Asus ProArt StudioBook 16 OLED",
			desc:  "Creator workstation with a 16-inch 4K OLED HDR display, NVIDIA RTX A3000, and Asus Dial for precise creative control.",
			brand: "Asus", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1517336714731-489689fd1ca8?w=600"]`,
			attrs:    `{"color":"Star Black","storage":"1TB SSD","ram":"32GB","display":"16 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},

		// Acer (4)
		{
			sku: "LAP-ACER-SWIFT5", name: "Acer Swift 5",
			desc:  "Featherlight 14-inch ultrabook under 1 kg with Intel Evo certification and an antimicrobial Corning Gorilla Glass touchscreen.",
			brand: "Acer", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1496181133206-80ce9b88a853?w=600"]`,
			attrs:    `{"color":"Mist Green","storage":"512GB SSD","ram":"16GB","display":"14 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-ACER-PH16", name: "Acer Predator Helios 16",
			desc:  "Beast-mode gaming laptop with RTX 4080, 240Hz Mini-LED display, and 5th Gen AeroBlade 3D fan technology.",
			brand: "Acer", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1603302576837-37561b2e2302?w=600"]`,
			attrs:    `{"color":"Abyssal Black","storage":"1TB SSD","ram":"32GB","display":"16 inch","type":"gaming"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-ACER-ASP5", name: "Acer Aspire 5",
			desc:  "Solid mid-range laptop with a 15.6-inch IPS display and AMD Ryzen 5 for dependable everyday performance.",
			brand: "Acer", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=600"]`,
			attrs:    `{"color":"Steel Gray","storage":"256GB SSD","ram":"8GB","display":"15.6 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-ACER-TMP2", name: "Acer TravelMate P2",
			desc:  "Business-ready laptop with a durable chassis, TPM 2.0 security, and comfortable typing experience for road warriors.",
			brand: "Acer", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=600"]`,
			attrs:    `{"color":"Shale Black","storage":"512GB SSD","ram":"16GB","display":"14 inch","type":"business"}`,
			ownerSlug: "",
		},

		// MSI (4)
		{
			sku: "LAP-MSI-CRTZ16", name: "MSI Creator Z16",
			desc:  "Premium creator laptop with a 16-inch QHD+ display calibrated to 100% DCI-P3 and Intel Core i9 performance.",
			brand: "MSI", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1517336714731-489689fd1ca8?w=600"]`,
			attrs:    `{"color":"Lunar Gray","storage":"1TB SSD","ram":"32GB","display":"16 inch","type":"ultrabook"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-MSI-STEALTH16", name: "MSI Stealth 16 Studio",
			desc:  "Slim gaming laptop that blends RTX 4070 graphics with a professional design, perfect for work and play.",
			brand: "MSI", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1603302576837-37561b2e2302?w=600"]`,
			attrs:    `{"color":"Star Blue","storage":"1TB SSD","ram":"16GB","display":"16 inch","type":"gaming"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-MSI-MOD15", name: "MSI Modern 15",
			desc:  "Sleek and lightweight 15.6-inch business laptop with Intel Core i7 and a minimalist aesthetic for professionals.",
			brand: "MSI", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1496181133206-80ce9b88a853?w=600"]`,
			attrs:    `{"color":"Classic Black","storage":"512GB SSD","ram":"16GB","display":"15.6 inch","type":"business"}`,
			ownerSlug: "",
		},
		{
			sku: "LAP-MSI-RAIDER78", name: "MSI Raider GE78 HX",
			desc:  "Flagship gaming laptop with RTX 4090, Intel Core i9-13980HX, and a massive 18-inch UHD+ Mini-LED display.",
			brand: "MSI", catSlug: "laptops",
			images:   `["https://images.unsplash.com/photo-1603302576837-37561b2e2302?w=600"]`,
			attrs:    `{"color":"Titanium Gray","storage":"2TB SSD","ram":"64GB","display":"18 inch","type":"gaming"}`,
			ownerSlug: "",
		},

		// ───────────────────────────── Headphones (20) ──────────────────────────

		// Apple (3)
		{
			sku: "HEAD-APPLE-APP2", name: "Apple AirPods Pro 2",
			desc:  "Second-generation AirPods Pro with adaptive transparency, personalised spatial audio, and USB-C charging case.",
			brand: "Apple", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1600294037681-c80b4cb5b434?w=600"]`,
			attrs:    `{"color":"White","type":"tws","features":"ANC, spatial audio, adaptive transparency"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-APPLE-AMAX", name: "Apple AirPods Max",
			desc:  "Premium over-ear headphones with high-fidelity audio, computational audio processing, and a stunning aluminium design.",
			brand: "Apple", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1625245488600-f03fef636a3c?w=600"]`,
			attrs:    `{"color":"Space Gray","type":"over-ear","features":"ANC, spatial audio, wireless"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-APPLE-AP3", name: "Apple AirPods 3rd Gen",
			desc:  "Redesigned AirPods with spatial audio, sweat resistance, and up to 6 hours of listening time on a single charge.",
			brand: "Apple", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1600294037681-c80b4cb5b434?w=600"]`,
			attrs:    `{"color":"White","type":"tws","features":"spatial audio, wireless, sweat resistant"}`,
			ownerSlug: "",
		},

		// Sony (4)
		{
			sku: "HEAD-SONY-WH1KXM5", name: "Sony WH-1000XM5",
			desc:  "Industry-leading noise cancelling over-ear headphones with 30 hours of battery and exceptional call clarity.",
			brand: "Sony", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=600"]`,
			attrs:    `{"color":"Black","type":"over-ear","features":"ANC, wireless, LDAC, multipoint"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-SONY-WF1KXM5", name: "Sony WF-1000XM5",
			desc:  "World's smallest and lightest noise-cancelling true wireless earbuds with exceptional sound quality.",
			brand: "Sony", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=600"]`,
			attrs:    `{"color":"Platinum Silver","type":"tws","features":"ANC, wireless, LDAC, Hi-Res"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-SONY-WHCH720N", name: "Sony WH-CH720N",
			desc:  "Lightweight over-ear headphones with noise cancelling, 50 hours of battery life, and comfortable fit for all-day wear.",
			brand: "Sony", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=600"]`,
			attrs:    `{"color":"Blue","type":"over-ear","features":"ANC, wireless, multipoint"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-SONY-LINKBUDS", name: "Sony LinkBuds S",
			desc:  "Compact true wireless earbuds with adaptive sound control that automatically switches between ANC and ambient modes.",
			brand: "Sony", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=600"]`,
			attrs:    `{"color":"Earth Blue","type":"tws","features":"ANC, wireless, adaptive sound"}`,
			ownerSlug: "",
		},

		// Bose (3)
		{
			sku: "HEAD-BOSE-QCULTRA", name: "Bose QuietComfort Ultra Headphones",
			desc:  "Flagship noise cancelling headphones with immersive spatial audio and world-class quiet for total immersion.",
			brand: "Bose", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=600"]`,
			attrs:    `{"color":"Black","type":"over-ear","features":"ANC, spatial audio, wireless"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-BOSE-QCEB2", name: "Bose QuietComfort Earbuds II",
			desc:  "Best-in-class noise cancelling earbuds with CustomTune technology that adapts sound to your ear shape.",
			brand: "Bose", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=600"]`,
			attrs:    `{"color":"Soapstone","type":"tws","features":"ANC, wireless, CustomTune"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-BOSE-SLFLEX", name: "Bose SoundLink Flex",
			desc:  "Rugged portable Bluetooth speaker with deep, rich sound that travels wherever you do — waterproof and dustproof.",
			brand: "Bose", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1608043152269-423dbba4e7e1?w=600"]`,
			attrs:    `{"color":"Stone Blue","type":"in-ear","features":"wireless, waterproof, portable"}`,
			ownerSlug: "",
		},

		// Sennheiser (3)
		{
			sku: "HEAD-SENN-MOM4", name: "Sennheiser Momentum 4 Wireless",
			desc:  "Audiophile-grade wireless headphones with 60 hours of battery, adaptive ANC, and premium leather-and-metal build.",
			brand: "Sennheiser", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=600"]`,
			attrs:    `{"color":"Black Copper","type":"over-ear","features":"ANC, wireless, aptX adaptive"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-SENN-HD660S2", name: "Sennheiser HD 660S2",
			desc:  "Open-back audiophile headphones with refined transducers for an even more natural and detailed listening experience.",
			brand: "Sennheiser", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=600"]`,
			attrs:    `{"color":"Black","type":"over-ear","features":"wired, open-back, audiophile"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-SENN-MTW3", name: "Sennheiser Momentum True Wireless 3",
			desc:  "Premium true wireless earbuds with adaptive ANC, aptX Adaptive codec, and a sleek fabric charging case.",
			brand: "Sennheiser", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=600"]`,
			attrs:    `{"color":"Graphite","type":"tws","features":"ANC, wireless, aptX adaptive"}`,
			ownerSlug: "",
		},

		// JBL (4)
		{
			sku: "HEAD-JBL-TPRO2", name: "JBL Tour Pro 2",
			desc:  "True wireless earbuds featuring a smart charging case with a touchscreen display for managing calls, music, and ANC.",
			brand: "JBL", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=600"]`,
			attrs:    `{"color":"Black","type":"tws","features":"ANC, wireless, smart case display"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-JBL-TBEAM", name: "JBL Tune Beam",
			desc:  "Affordable true wireless earbuds with JBL Pure Bass sound, active noise cancelling, and 48 hours total playtime.",
			brand: "JBL", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=600"]`,
			attrs:    `{"color":"Blue","type":"tws","features":"ANC, wireless, Pure Bass"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-JBL-LIVE770", name: "JBL Live 770NC",
			desc:  "Over-ear wireless headphones with adaptive noise cancelling, spatial sound, and a comfortable over-ear fit for long sessions.",
			brand: "JBL", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=600"]`,
			attrs:    `{"color":"Black","type":"over-ear","features":"ANC, wireless, spatial sound"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-JBL-RFPRO", name: "JBL Reflect Flow Pro",
			desc:  "Sport true wireless earbuds with IP68 waterproof rating, adaptive ANC, and secure-fit wings for intense workouts.",
			brand: "JBL", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=600"]`,
			attrs:    `{"color":"Black","type":"tws","features":"ANC, wireless, IP68, sport"}`,
			ownerSlug: "",
		},

		// Marshall (3)
		{
			sku: "HEAD-MARSH-MON2", name: "Marshall Monitor II ANC",
			desc:  "Iconic over-ear headphones with active noise cancelling, 30 hours of wireless playtime, and signature Marshall sound.",
			brand: "Marshall", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=600"]`,
			attrs:    `{"color":"Black","type":"over-ear","features":"ANC, wireless, M-button customisable"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-MARSH-MAJ4", name: "Marshall Major IV",
			desc:  "On-ear wireless headphones with 80+ hours of battery life, wireless charging, and the classic Marshall look.",
			brand: "Marshall", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=600"]`,
			attrs:    `{"color":"Brown","type":"over-ear","features":"wireless, wireless charging"}`,
			ownerSlug: "",
		},
		{
			sku: "HEAD-MARSH-MIN3", name: "Marshall Minor III",
			desc:  "True wireless earbuds with 12mm custom-tuned drivers, 25 hours of total battery, and a compact vintage-style case.",
			brand: "Marshall", catSlug: "headphones",
			images:   `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=600"]`,
			attrs:    `{"color":"Black","type":"tws","features":"wireless, 12mm drivers, quick charge"}`,
			ownerSlug: "",
		},

		// ───────────────────────────── Cameras (15) ─────────────────────────────

		// Sony (4)
		{
			sku: "CAM-SONY-A7IV", name: "Sony Alpha A7 IV",
			desc:  "Versatile full-frame mirrorless camera with 33MP sensor, real-time tracking AF, and 4K 60p video recording.",
			brand: "Sony", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1516035069371-29a1b244cc32?w=600"]`,
			attrs:    `{"color":"Black","type":"mirrorless","sensor":"full-frame"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-SONY-A7CII", name: "Sony Alpha A7C II",
			desc:  "Compact full-frame mirrorless camera with 33MP sensor and AI-based autofocus in a remarkably lightweight body.",
			brand: "Sony", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1516035069371-29a1b244cc32?w=600"]`,
			attrs:    `{"color":"Silver","type":"mirrorless","sensor":"full-frame"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-SONY-ZVE10II", name: "Sony ZV-E10 II",
			desc:  "Next-gen vlog camera with interchangeable lenses, 4K 60p, and cinematic vlog settings for content creators.",
			brand: "Sony", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1502920917128-1aa500764cbd?w=600"]`,
			attrs:    `{"color":"Black","type":"mirrorless","sensor":"APS-C"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-SONY-RX100VII", name: "Sony RX100 VII",
			desc:  "Pocket-sized premium compact camera with a 1-inch sensor, 20fps burst shooting, and real-time eye AF.",
			brand: "Sony", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1502920917128-1aa500764cbd?w=600"]`,
			attrs:    `{"color":"Black","type":"compact","sensor":"1-inch"}`,
			ownerSlug: "",
		},

		// Canon (4)
		{
			sku: "CAM-CANON-R6II", name: "Canon EOS R6 Mark II",
			desc:  "High-speed full-frame mirrorless camera with 24.2MP sensor, up to 40fps electronic shutter, and in-body stabilisation.",
			brand: "Canon", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1516035069371-29a1b244cc32?w=600"]`,
			attrs:    `{"color":"Black","type":"mirrorless","sensor":"full-frame"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-CANON-R8", name: "Canon EOS R8",
			desc:  "Lightweight full-frame mirrorless camera with advanced AF and 4K 60p video — ideal for hybrid shooters.",
			brand: "Canon", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1516035069371-29a1b244cc32?w=600"]`,
			attrs:    `{"color":"Black","type":"mirrorless","sensor":"full-frame"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-CANON-R50", name: "Canon EOS R50",
			desc:  "Entry-level mirrorless camera with 24.2MP APS-C sensor, beginner-friendly UI, and compact form factor.",
			brand: "Canon", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1502920917128-1aa500764cbd?w=600"]`,
			attrs:    `{"color":"White","type":"mirrorless","sensor":"APS-C"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-CANON-PSV10", name: "Canon PowerShot V10",
			desc:  "Ultra-compact vlog camera with a built-in stand, wide-angle lens, and one-touch video recording for casual creators.",
			brand: "Canon", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1502920917128-1aa500764cbd?w=600"]`,
			attrs:    `{"color":"Silver","type":"compact","sensor":"1-inch"}`,
			ownerSlug: "",
		},

		// Nikon (3)
		{
			sku: "CAM-NIKON-Z8", name: "Nikon Z8",
			desc:  "Professional full-frame mirrorless camera with a 45.7MP sensor, 8K video, and EXPEED 7 processing engine.",
			brand: "Nikon", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1516035069371-29a1b244cc32?w=600"]`,
			attrs:    `{"color":"Black","type":"mirrorless","sensor":"full-frame"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-NIKON-Z5", name: "Nikon Z5",
			desc:  "Affordable full-frame mirrorless camera with dual card slots, weather sealing, and 4K UHD video capability.",
			brand: "Nikon", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1516035069371-29a1b244cc32?w=600"]`,
			attrs:    `{"color":"Black","type":"mirrorless","sensor":"full-frame"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-NIKON-Z30", name: "Nikon Z30",
			desc:  "Compact APS-C mirrorless camera designed for vloggers with a flip-out screen and always-on recording indicator.",
			brand: "Nikon", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1502920917128-1aa500764cbd?w=600"]`,
			attrs:    `{"color":"Black","type":"mirrorless","sensor":"APS-C"}`,
			ownerSlug: "",
		},

		// Fujifilm (2)
		{
			sku: "CAM-FUJI-XT5", name: "Fujifilm X-T5",
			desc:  "Retro-styled APS-C mirrorless camera with 40.2MP sensor, in-body stabilisation, and iconic film simulation modes.",
			brand: "Fujifilm", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1516035069371-29a1b244cc32?w=600"]`,
			attrs:    `{"color":"Silver","type":"mirrorless","sensor":"APS-C"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-FUJI-XS20", name: "Fujifilm X-S20",
			desc:  "Versatile APS-C camera with 6.2K video, subject-detection AF, and up to 800 shots per charge for all-day shooting.",
			brand: "Fujifilm", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1502920917128-1aa500764cbd?w=600"]`,
			attrs:    `{"color":"Black","type":"mirrorless","sensor":"APS-C"}`,
			ownerSlug: "",
		},

		// GoPro (2)
		{
			sku: "CAM-GOPRO-H12", name: "GoPro Hero 12 Black",
			desc:  "Flagship action camera with HyperSmooth 6.0 stabilisation, 5.3K60 video, and Max Lens Mod 2.0 compatibility.",
			brand: "GoPro", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1526170375885-4d8ecf77b99f?w=600"]`,
			attrs:    `{"color":"Black","type":"action","sensor":"1/1.9-inch"}`,
			ownerSlug: "",
		},
		{
			sku: "CAM-GOPRO-H11MINI", name: "GoPro Hero 11 Mini",
			desc:  "Ultra-compact action camera with the same sensor as Hero 11 Black in a rugged, pocketable, mount-ready form factor.",
			brand: "GoPro", catSlug: "cameras",
			images:   `["https://images.unsplash.com/photo-1526170375885-4d8ecf77b99f?w=600"]`,
			attrs:    `{"color":"Black","type":"action","sensor":"1/1.9-inch"}`,
			ownerSlug: "",
		},
	}
}

func seedElecAudioListings() []listing {
	return []listing{
		// ───────────────────────────── Laptops ───────────────────────────────────

		// Apple MacBook Pro 16 M3 Max
		{tenantSlug: "techstore", mpSKU: "LAP-APPLE-MBP16M3MAX", price: 34999000, stock: 8, rating: 4.9},
		{tenantSlug: "fashionhub", mpSKU: "LAP-APPLE-MBP16M3MAX", price: 35499000, stock: 4, rating: 4.8},
		// Apple MacBook Pro 14 M3 Pro
		{tenantSlug: "techstore", mpSKU: "LAP-APPLE-MBP14M3PRO", price: 24999000, stock: 12, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "LAP-APPLE-MBP14M3PRO", price: 25499000, stock: 6, rating: 4.7},
		// Apple MacBook Air 15 M3
		{tenantSlug: "techstore", mpSKU: "LAP-APPLE-MBA15M3", price: 17999000, stock: 18, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "LAP-APPLE-MBA15M3", price: 18499000, stock: 10, rating: 4.7},
		// Apple MacBook Air 13 M3
		{tenantSlug: "techstore", mpSKU: "LAP-APPLE-MBA13M3", price: 12999000, stock: 25, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "LAP-APPLE-MBA13M3", price: 13499000, stock: 15, rating: 4.6},
		// Apple MacBook Pro 13 M2
		{tenantSlug: "techstore", mpSKU: "LAP-APPLE-MBP13M2", price: 14999000, stock: 14, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "LAP-APPLE-MBP13M2", price: 15499000, stock: 7, rating: 4.5},

		// Dell XPS 15
		{tenantSlug: "techstore", mpSKU: "LAP-DELL-XPS15", price: 17999000, stock: 15, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "LAP-DELL-XPS15", price: 18299000, stock: 8, rating: 4.6},
		// Dell XPS 13
		{tenantSlug: "techstore", mpSKU: "LAP-DELL-XPS13", price: 10999000, stock: 20, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "LAP-DELL-XPS13", price: 11299000, stock: 10, rating: 4.5},
		// Dell Inspiron 16
		{tenantSlug: "techstore", mpSKU: "LAP-DELL-INSP16", price: 7999000, stock: 22, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "LAP-DELL-INSP16", price: 8299000, stock: 12, rating: 4.3},
		// Dell Latitude 14
		{tenantSlug: "techstore", mpSKU: "LAP-DELL-LAT14", price: 10499000, stock: 10, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "LAP-DELL-LAT14", price: 10799000, stock: 5, rating: 4.4},

		// Lenovo ThinkPad X1 Carbon
		{tenantSlug: "techstore", mpSKU: "LAP-LEN-X1CARBON", price: 19999000, stock: 10, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "LAP-LEN-X1CARBON", price: 20499000, stock: 5, rating: 4.7},
		// Lenovo ThinkPad T14
		{tenantSlug: "techstore", mpSKU: "LAP-LEN-T14", price: 9999000, stock: 16, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "LAP-LEN-T14", price: 10299000, stock: 8, rating: 4.4},
		// Lenovo IdeaPad 5
		{tenantSlug: "techstore", mpSKU: "LAP-LEN-IDEAPAD5", price: 5999000, stock: 30, rating: 4.3},
		{tenantSlug: "fashionhub", mpSKU: "LAP-LEN-IDEAPAD5", price: 6199000, stock: 20, rating: 4.2},
		// Lenovo Legion Pro 5
		{tenantSlug: "techstore", mpSKU: "LAP-LEN-LEGPRO5", price: 16999000, stock: 8, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "LAP-LEN-LEGPRO5", price: 17499000, stock: 4, rating: 4.6},
		// Lenovo Yoga 9i
		{tenantSlug: "techstore", mpSKU: "LAP-LEN-YOGA9I", price: 14999000, stock: 12, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "LAP-LEN-YOGA9I", price: 15399000, stock: 6, rating: 4.6},

		// HP Spectre x360
		{tenantSlug: "techstore", mpSKU: "LAP-HP-SPECX360", price: 16999000, stock: 10, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "LAP-HP-SPECX360", price: 17399000, stock: 5, rating: 4.6},
		// HP Pavilion 15
		{tenantSlug: "techstore", mpSKU: "LAP-HP-PAV15", price: 6499000, stock: 28, rating: 4.3},
		{tenantSlug: "fashionhub", mpSKU: "LAP-HP-PAV15", price: 6799000, stock: 15, rating: 4.2},
		// HP EliteBook 840
		{tenantSlug: "techstore", mpSKU: "LAP-HP-EB840", price: 12999000, stock: 10, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "LAP-HP-EB840", price: 13299000, stock: 5, rating: 4.5},
		// HP Omen 16
		{tenantSlug: "techstore", mpSKU: "LAP-HP-OMEN16", price: 14499000, stock: 9, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "LAP-HP-OMEN16", price: 14899000, stock: 4, rating: 4.5},

		// Asus ZenBook 14
		{tenantSlug: "techstore", mpSKU: "LAP-ASUS-ZB14", price: 10999000, stock: 16, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "LAP-ASUS-ZB14", price: 11299000, stock: 8, rating: 4.5},
		// Asus ROG Strix G16
		{tenantSlug: "techstore", mpSKU: "LAP-ASUS-ROGG16", price: 15999000, stock: 7, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "LAP-ASUS-ROGG16", price: 16399000, stock: 3, rating: 4.6},
		// Asus VivoBook 15
		{tenantSlug: "techstore", mpSKU: "LAP-ASUS-VB15", price: 4999000, stock: 35, rating: 4.2},
		{tenantSlug: "fashionhub", mpSKU: "LAP-ASUS-VB15", price: 5199000, stock: 20, rating: 4.1},
		// Asus ProArt StudioBook
		{tenantSlug: "techstore", mpSKU: "LAP-ASUS-PROART", price: 24999000, stock: 5, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "LAP-ASUS-PROART", price: 25499000, stock: 3, rating: 4.7},

		// Acer Swift 5
		{tenantSlug: "techstore", mpSKU: "LAP-ACER-SWIFT5", price: 9999000, stock: 14, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "LAP-ACER-SWIFT5", price: 10299000, stock: 7, rating: 4.4},
		// Acer Predator Helios 16
		{tenantSlug: "techstore", mpSKU: "LAP-ACER-PH16", price: 19999000, stock: 6, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "LAP-ACER-PH16", price: 20499000, stock: 3, rating: 4.6},
		// Acer Aspire 5
		{tenantSlug: "techstore", mpSKU: "LAP-ACER-ASP5", price: 4999000, stock: 32, rating: 4.2},
		{tenantSlug: "fashionhub", mpSKU: "LAP-ACER-ASP5", price: 5199000, stock: 18, rating: 4.1},
		// Acer TravelMate P2
		{tenantSlug: "techstore", mpSKU: "LAP-ACER-TMP2", price: 7499000, stock: 12, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "LAP-ACER-TMP2", price: 7799000, stock: 6, rating: 4.3},

		// MSI Creator Z16
		{tenantSlug: "techstore", mpSKU: "LAP-MSI-CRTZ16", price: 19999000, stock: 6, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "LAP-MSI-CRTZ16", price: 20499000, stock: 3, rating: 4.6},
		// MSI Stealth 16 Studio
		{tenantSlug: "techstore", mpSKU: "LAP-MSI-STEALTH16", price: 17999000, stock: 7, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "LAP-MSI-STEALTH16", price: 18499000, stock: 4, rating: 4.6},
		// MSI Modern 15
		{tenantSlug: "techstore", mpSKU: "LAP-MSI-MOD15", price: 7499000, stock: 18, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "LAP-MSI-MOD15", price: 7799000, stock: 9, rating: 4.3},
		// MSI Raider GE78 HX
		{tenantSlug: "techstore", mpSKU: "LAP-MSI-RAIDER78", price: 29999000, stock: 4, rating: 4.9},
		{tenantSlug: "fashionhub", mpSKU: "LAP-MSI-RAIDER78", price: 30499000, stock: 3, rating: 4.8},

		// ───────────────────────────── Headphones ────────────────────────────────

		// Apple AirPods Pro 2
		{tenantSlug: "techstore", mpSKU: "HEAD-APPLE-APP2", price: 24990_00, stock: 40, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-APPLE-APP2", price: 25990_00, stock: 25, rating: 4.7},
		// Apple AirPods Max
		{tenantSlug: "techstore", mpSKU: "HEAD-APPLE-AMAX", price: 59990_00, stock: 10, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-APPLE-AMAX", price: 61990_00, stock: 5, rating: 4.6},
		// Apple AirPods 3
		{tenantSlug: "techstore", mpSKU: "HEAD-APPLE-AP3", price: 17990_00, stock: 35, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-APPLE-AP3", price: 18490_00, stock: 20, rating: 4.4},

		// Sony WH-1000XM5
		{tenantSlug: "techstore", mpSKU: "HEAD-SONY-WH1KXM5", price: 34990_00, stock: 20, rating: 4.9},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-SONY-WH1KXM5", price: 35990_00, stock: 12, rating: 4.8},
		// Sony WF-1000XM5
		{tenantSlug: "techstore", mpSKU: "HEAD-SONY-WF1KXM5", price: 27990_00, stock: 18, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-SONY-WF1KXM5", price: 28990_00, stock: 10, rating: 4.7},
		// Sony WH-CH720N
		{tenantSlug: "techstore", mpSKU: "HEAD-SONY-WHCH720N", price: 7990_00, stock: 30, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-SONY-WHCH720N", price: 8490_00, stock: 18, rating: 4.3},
		// Sony LinkBuds S
		{tenantSlug: "techstore", mpSKU: "HEAD-SONY-LINKBUDS", price: 14990_00, stock: 22, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-SONY-LINKBUDS", price: 15490_00, stock: 12, rating: 4.4},

		// Bose QuietComfort Ultra
		{tenantSlug: "techstore", mpSKU: "HEAD-BOSE-QCULTRA", price: 39990_00, stock: 12, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-BOSE-QCULTRA", price: 40990_00, stock: 6, rating: 4.7},
		// Bose QC Earbuds II
		{tenantSlug: "techstore", mpSKU: "HEAD-BOSE-QCEB2", price: 24990_00, stock: 16, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-BOSE-QCEB2", price: 25990_00, stock: 8, rating: 4.6},
		// Bose SoundLink Flex
		{tenantSlug: "techstore", mpSKU: "HEAD-BOSE-SLFLEX", price: 12990_00, stock: 20, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-BOSE-SLFLEX", price: 13490_00, stock: 10, rating: 4.4},

		// Sennheiser Momentum 4
		{tenantSlug: "techstore", mpSKU: "HEAD-SENN-MOM4", price: 29990_00, stock: 14, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-SENN-MOM4", price: 30990_00, stock: 7, rating: 4.7},
		// Sennheiser HD 660S2
		{tenantSlug: "techstore", mpSKU: "HEAD-SENN-HD660S2", price: 34990_00, stock: 8, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-SENN-HD660S2", price: 35990_00, stock: 4, rating: 4.7},
		// Sennheiser MTW 3
		{tenantSlug: "techstore", mpSKU: "HEAD-SENN-MTW3", price: 19990_00, stock: 16, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-SENN-MTW3", price: 20990_00, stock: 8, rating: 4.5},

		// JBL Tour Pro 2
		{tenantSlug: "techstore", mpSKU: "HEAD-JBL-TPRO2", price: 19990_00, stock: 18, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-JBL-TPRO2", price: 20490_00, stock: 10, rating: 4.5},
		// JBL Tune Beam
		{tenantSlug: "techstore", mpSKU: "HEAD-JBL-TBEAM", price: 5990_00, stock: 35, rating: 4.3},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-JBL-TBEAM", price: 6290_00, stock: 20, rating: 4.2},
		// JBL Live 770NC
		{tenantSlug: "techstore", mpSKU: "HEAD-JBL-LIVE770", price: 12990_00, stock: 20, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-JBL-LIVE770", price: 13490_00, stock: 10, rating: 4.4},
		// JBL Reflect Flow Pro
		{tenantSlug: "techstore", mpSKU: "HEAD-JBL-RFPRO", price: 11990_00, stock: 22, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-JBL-RFPRO", price: 12490_00, stock: 12, rating: 4.3},

		// Marshall Monitor II ANC
		{tenantSlug: "techstore", mpSKU: "HEAD-MARSH-MON2", price: 24990_00, stock: 12, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-MARSH-MON2", price: 25490_00, stock: 6, rating: 4.5},
		// Marshall Major IV
		{tenantSlug: "techstore", mpSKU: "HEAD-MARSH-MAJ4", price: 8990_00, stock: 25, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-MARSH-MAJ4", price: 9290_00, stock: 14, rating: 4.4},
		// Marshall Minor III
		{tenantSlug: "techstore", mpSKU: "HEAD-MARSH-MIN3", price: 7990_00, stock: 28, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "HEAD-MARSH-MIN3", price: 8290_00, stock: 15, rating: 4.3},

		// ───────────────────────────── Cameras ───────────────────────────────────

		// Sony Alpha A7 IV
		{tenantSlug: "techstore", mpSKU: "CAM-SONY-A7IV", price: 24999000, stock: 8, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "CAM-SONY-A7IV", price: 25499000, stock: 4, rating: 4.7},
		// Sony Alpha A7C II
		{tenantSlug: "techstore", mpSKU: "CAM-SONY-A7CII", price: 19999000, stock: 10, rating: 4.7},
		{tenantSlug: "fashionhub", mpSKU: "CAM-SONY-A7CII", price: 20499000, stock: 5, rating: 4.6},
		// Sony ZV-E10 II
		{tenantSlug: "techstore", mpSKU: "CAM-SONY-ZVE10II", price: 8999000, stock: 15, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "CAM-SONY-ZVE10II", price: 9299000, stock: 8, rating: 4.5},
		// Sony RX100 VII
		{tenantSlug: "techstore", mpSKU: "CAM-SONY-RX100VII", price: 10999000, stock: 12, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "CAM-SONY-RX100VII", price: 11299000, stock: 6, rating: 4.5},

		// Canon EOS R6 II
		{tenantSlug: "techstore", mpSKU: "CAM-CANON-R6II", price: 21999000, stock: 8, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "CAM-CANON-R6II", price: 22499000, stock: 4, rating: 4.7},
		// Canon EOS R8
		{tenantSlug: "techstore", mpSKU: "CAM-CANON-R8", price: 14999000, stock: 12, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "CAM-CANON-R8", price: 15499000, stock: 6, rating: 4.5},
		// Canon EOS R50
		{tenantSlug: "techstore", mpSKU: "CAM-CANON-R50", price: 5999000, stock: 20, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "CAM-CANON-R50", price: 6299000, stock: 12, rating: 4.3},
		// Canon PowerShot V10
		{tenantSlug: "techstore", mpSKU: "CAM-CANON-PSV10", price: 3999000, stock: 18, rating: 4.3},
		{tenantSlug: "fashionhub", mpSKU: "CAM-CANON-PSV10", price: 4199000, stock: 10, rating: 4.2},

		// Nikon Z8
		{tenantSlug: "techstore", mpSKU: "CAM-NIKON-Z8", price: 34999000, stock: 5, rating: 4.9},
		{tenantSlug: "fashionhub", mpSKU: "CAM-NIKON-Z8", price: 35499000, stock: 3, rating: 4.8},
		// Nikon Z5
		{tenantSlug: "techstore", mpSKU: "CAM-NIKON-Z5", price: 10999000, stock: 14, rating: 4.5},
		{tenantSlug: "fashionhub", mpSKU: "CAM-NIKON-Z5", price: 11299000, stock: 7, rating: 4.4},
		// Nikon Z30
		{tenantSlug: "techstore", mpSKU: "CAM-NIKON-Z30", price: 7499000, stock: 16, rating: 4.4},
		{tenantSlug: "fashionhub", mpSKU: "CAM-NIKON-Z30", price: 7799000, stock: 8, rating: 4.3},

		// Fujifilm X-T5
		{tenantSlug: "techstore", mpSKU: "CAM-FUJI-XT5", price: 16999000, stock: 10, rating: 4.8},
		{tenantSlug: "fashionhub", mpSKU: "CAM-FUJI-XT5", price: 17499000, stock: 5, rating: 4.7},
		// Fujifilm X-S20
		{tenantSlug: "techstore", mpSKU: "CAM-FUJI-XS20", price: 12999000, stock: 12, rating: 4.6},
		{tenantSlug: "fashionhub", mpSKU: "CAM-FUJI-XS20", price: 13299000, stock: 6, rating: 4.5},

		// GoPro Hero 12 — action cam → sportmaster as second tenant
		{tenantSlug: "techstore", mpSKU: "CAM-GOPRO-H12", price: 4499000, stock: 22, rating: 4.6},
		{tenantSlug: "sportmaster", mpSKU: "CAM-GOPRO-H12", price: 4699000, stock: 15, rating: 4.5},
		// GoPro Hero 11 Mini — action cam → sportmaster as second tenant
		{tenantSlug: "techstore", mpSKU: "CAM-GOPRO-H11MINI", price: 3299000, stock: 18, rating: 4.4},
		{tenantSlug: "sportmaster", mpSKU: "CAM-GOPRO-H11MINI", price: 3499000, stock: 10, rating: 4.3},
	}
}
