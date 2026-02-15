package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// --- Flags ---

var (
	flagURL         = flag.String("url", "https://www.heybabescosmetics.com", "Base URL of the site")
	flagOutputDir   = flag.String("outdir", "/Users/starknight/Keepstar_one_ultra/project_admin/Crawler_results", "Output directory")
	flagLimit       = flag.Int("limit", 1000, "Max products to crawl")
	flagConcurrency = flag.Int("concurrency", 5, "Parallel requests")
	flagDelay       = flag.Int("delay", 200, "Delay between requests in ms")
)

// --- Sitemap ---

type SitemapIndex struct {
	Sitemaps []SitemapEntry `xml:"sitemap"`
}

type SitemapEntry struct {
	Loc string `xml:"loc"`
}

type URLSet struct {
	URLs []URLEntry `xml:"url"`
}

type URLEntry struct {
	Loc string `xml:"loc"`
}

// --- JSON-LD ---

type JSONLDProduct struct {
	Type         string        `json:"@type"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	SKU          string        `json:"sku"`
	Brand        *JSONLDBrand  `json:"brand"`
	Image        interface{}   `json:"image"`
	Offers       *JSONLDOffers `json:"offers"`
	AggregateRat *JSONLDRating `json:"aggregateRating"`
}

type JSONLDBrand struct {
	Name string `json:"name"`
}

type JSONLDOffers struct {
	Price         interface{} `json:"price"`
	PriceCurrency string      `json:"priceCurrency"`
	Availability  string      `json:"availability"`
}

type JSONLDRating struct {
	RatingValue float64 `json:"ratingValue"`
}

// --- Import (matches usecases.ImportItem) ---

type ImportItem struct {
	Type       string         `json:"type"`
	SKU        string         `json:"sku"`
	Name       string         `json:"name"`
	Brand      string         `json:"brand"`
	Category   string         `json:"category"`
	Price      int            `json:"price"`
	Currency   string         `json:"currency"`
	Stock      int            `json:"stock"`
	Rating     float64        `json:"rating"`
	Images     []string       `json:"images"`
	Attributes map[string]any `json:"attributes"`
	Tags       []string       `json:"tags"`
}

type ImportRequest struct {
	Products []ImportItem `json:"products"`
}

// --- HTTP ---

var client = &http.Client{Timeout: 30 * time.Second}

func fetchURL(url string) ([]byte, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "KeepstarCrawler/1.0")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// --- Regex ---

var (
	reJSONLD     = regexp.MustCompile(`(?s)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)
	reBreadcrumb = regexp.MustCompile(`breadcrumbs__item-name[^>]*>([^<]+)<`)
	reArticle    = regexp.MustCompile(`Артикул:\s*(\S+)`)
	reImgGallery = regexp.MustCompile(`(?s)<a[^>]*data-fancybox[^>]*href="([^"]+\.(jpg|jpeg|png|webp))"`)
	reHTMLTag    = regexp.MustCompile(`<[^>]+>`)
	reWhitespace = regexp.MustCompile(`\s+`)
	reAccordIng  = regexp.MustCompile(`(?s)data-target="row_PROP_[^"]*"[^>]*>(.*?)</div>`)
	reAccordUse  = regexp.MustCompile(`(?s)data-target="row_USE"[^>]*>(.*?)</div>`)
	reDescHTML   = regexp.MustCompile(`(?s)itemprop="description"[^>]*>(.*?)</div>`)
	reVolShort   = regexp.MustCompile(`(?i)(\d+)\s*(?:мл|ml|гр|г)`)
)

// --- Sitemap pipeline ---

func fetchProductURLs(baseURL string) ([]string, error) {
	sitemapURL := baseURL + "/sitemap.xml"
	log.Printf("[sitemap] Fetching %s", sitemapURL)
	body, err := fetchURL(sitemapURL)
	if err != nil {
		return nil, err
	}

	// Parse sitemap index → find iblock-58
	var idx SitemapIndex
	if err := xml.Unmarshal(body, &idx); err != nil {
		return nil, err
	}

	var productSitemapURL string
	for _, s := range idx.Sitemaps {
		if strings.Contains(s.Loc, "iblock-58") {
			productSitemapURL = s.Loc
			break
		}
	}
	if productSitemapURL == "" {
		return nil, fmt.Errorf("product sitemap (iblock-58) not found")
	}

	log.Printf("[sitemap] Fetching %s", productSitemapURL)
	body, err = fetchURL(productSitemapURL)
	if err != nil {
		return nil, err
	}

	var urlset URLSet
	if err := xml.Unmarshal(body, &urlset); err != nil {
		return nil, err
	}

	var urls []string
	for _, u := range urlset.URLs {
		if strings.Contains(u.Loc, "/catalog/") && !strings.HasSuffix(u.Loc, "/catalog/") {
			urls = append(urls, u.Loc)
		}
	}
	return urls, nil
}

// --- Product page parsing ---

func parseProductPage(pageURL, html string) *ImportItem {
	// 1. Find JSON-LD Product
	var product *JSONLDProduct
	for _, m := range reJSONLD.FindAllStringSubmatch(html, -1) {
		var ld JSONLDProduct
		if err := json.Unmarshal([]byte(m[1]), &ld); err != nil {
			continue
		}
		if ld.Type == "Product" || ld.Type == "http://schema.org/Product" {
			product = &ld
			break
		}
	}
	if product == nil {
		return nil
	}

	// 2. Build item
	item := &ImportItem{
		Type:       "product",
		Currency:   "RUB",
		Attributes: make(map[string]any),
	}

	item.Name = cleanText(product.Name)
	item.SKU = product.SKU
	if item.SKU == "" {
		if m := reArticle.FindStringSubmatch(html); m != nil {
			item.SKU = strings.TrimSpace(m[1])
		}
	}

	if product.Brand != nil {
		item.Brand = product.Brand.Name
	}

	if product.Offers != nil {
		item.Price = parsePrice(product.Offers.Price)
		if product.Offers.PriceCurrency != "" {
			item.Currency = product.Offers.PriceCurrency
		}
		if strings.Contains(product.Offers.Availability, "InStock") {
			item.Stock = 1
		}
	}

	if product.AggregateRat != nil {
		item.Rating = product.AggregateRat.RatingValue
	}

	// 3. Images — from JSON-LD + gallery
	item.Images = collectImages(product.Image, html, pageURL)

	// 4. Category from breadcrumbs
	// Page has breadcrumbs twice (HTML + JS NAV_CHAIN). Take only first set.
	// Pattern: Главная — Магазин — Cat1 — Cat2 — ... — ProductName
	allCrumbs := reBreadcrumb.FindAllStringSubmatch(html, -1)
	// Extract first set (stop at second "Главная")
	var crumbs []string
	for i, c := range allCrumbs {
		name := strings.TrimSpace(c[1])
		if i > 0 && name == "Главная" {
			break // second set started
		}
		crumbs = append(crumbs, name)
	}
	// Take between "Магазин" and last (product name)
	var catParts []string
	foundMagazin := false
	for _, name := range crumbs[:max(0, len(crumbs)-1)] { // skip last = product
		if name == "Магазин" {
			foundMagazin = true
			continue
		}
		if foundMagazin {
			catParts = append(catParts, name)
		}
	}
	if len(catParts) > 0 {
		item.Category = strings.Join(catParts, " > ")
	}

	// 5. Structured attributes from description
	// Prefer HTML description (itemprop) — has full structured sections
	// JSON-LD description is often truncated
	descText := ""
	if m := reDescHTML.FindStringSubmatch(html); m != nil {
		descText = cleanText(m[1])
	}
	if descText == "" && product.Description != "" {
		descText = cleanText(product.Description)
	}
	if descText != "" {
		sections := splitDescription(descText)
		for key, val := range sections {
			if val != "" {
				item.Attributes[key] = val
			}
		}
	}

	// 6. Accordion overrides: ingredients (INCI) and how_to_use
	if m := reAccordIng.FindStringSubmatch(html); m != nil {
		if inci := cleanText(m[1]); inci != "" {
			item.Attributes["ingredients"] = inci
		}
	}
	if m := reAccordUse.FindStringSubmatch(html); m != nil {
		if howTo := cleanText(m[1]); howTo != "" {
			item.Attributes["how_to_use"] = howTo
		}
	}

	// Volume fallback: from description text, then from product name
	if _, ok := item.Attributes["volume"]; !ok {
		if desc, ok := item.Attributes["description"].(string); ok {
			if m := reVolShort.FindString(desc); m != "" {
				item.Attributes["volume"] = m
			}
		}
	}
	if _, ok := item.Attributes["volume"]; !ok {
		if m := reVolShort.FindString(item.Name); m != "" {
			item.Attributes["volume"] = m
		}
	}

	// 7. Tags
	item.Tags = buildTags(item)

	return item
}

func parsePrice(v interface{}) int {
	switch p := v.(type) {
	case float64:
		return int(p * 100) // rubles → kopecks
	case string:
		p = strings.ReplaceAll(p, " ", "")
		p = strings.ReplaceAll(p, ",", ".")
		var f float64
		if _, err := fmt.Sscanf(p, "%f", &f); err == nil {
			return int(f * 100)
		}
	}
	return 0
}

func collectImages(jsonLDImage interface{}, html, pageURL string) []string {
	seen := make(map[string]bool)
	var imgs []string
	add := func(url string) {
		url = resolveURL(url, pageURL)
		lower := strings.ToLower(url)
		if seen[url] || strings.HasSuffix(lower, ".svg") || strings.Contains(lower, "/loaders/") {
			return
		}
		seen[url] = true
		imgs = append(imgs, url)
	}

	// From JSON-LD
	switch img := jsonLDImage.(type) {
	case string:
		add(img)
	case []interface{}:
		for _, s := range img {
			if u, ok := s.(string); ok {
				add(u)
			}
		}
	}

	// From HTML gallery (fancybox links)
	for _, m := range reImgGallery.FindAllStringSubmatch(html, -1) {
		add(m[1])
	}

	return imgs
}

func resolveURL(href, pageURL string) string {
	if strings.HasPrefix(href, "http") {
		return href
	}
	if idx := strings.Index(pageURL, "//"); idx >= 0 {
		rest := pageURL[idx+2:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			base := pageURL[:idx+2+slash]
			if strings.HasPrefix(href, "/") {
				return base + href
			}
			return base + "/" + href
		}
	}
	return href
}

func buildTags(item *ImportItem) []string {
	seen := make(map[string]bool)
	var tags []string
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && !seen[s] {
			seen[s] = true
			tags = append(tags, s)
		}
	}
	add(item.Brand)
	if item.Category != "" {
		parts := strings.Split(item.Category, " > ")
		add(parts[len(parts)-1]) // deepest category
	}
	return tags
}

// --- Description splitting ---

var sectionMarkers = []struct {
	key      string
	patterns []string
}{
	{"volume", []string{"Объём:", "Объем:"}},
	{"skin_type", []string{"Подходит для:", "Подойдет для:", "Подойдёт для:"}},
	{"benefits", []string{"Преимущества:"}},
	{"active_ingredients", []string{"Основные компоненты:"}},
	{"how_to_use", []string{"Способ применения:"}},
	{"ingredients", []string{"Состав:"}},
	{"_warnings", []string{"Предостережения:"}},
}

func splitDescription(desc string) map[string]string {
	type marker struct {
		key string
		pos int
		end int
	}

	var found []marker
	for _, sm := range sectionMarkers {
		for _, pat := range sm.patterns {
			idx := strings.Index(desc, pat)
			if idx >= 0 {
				found = append(found, marker{sm.key, idx, idx + len(pat)})
				break
			}
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].pos < found[j].pos })

	result := make(map[string]string)

	if len(found) > 0 {
		result["description"] = strings.TrimSpace(desc[:found[0].pos])
	} else {
		result["description"] = desc
	}

	for i, m := range found {
		end := len(desc)
		if i+1 < len(found) {
			end = found[i+1].pos
		}
		text := strings.TrimSpace(desc[m.end:end])
		if !strings.HasPrefix(m.key, "_") && text != "" {
			result[m.key] = text
		}
	}

	// Clean trailing period from volume
	if vol, ok := result["volume"]; ok {
		result["volume"] = strings.TrimRight(vol, ". ")
	}

	return result
}

func cleanText(s string) string {
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = reHTMLTag.ReplaceAllString(s, " ")
	s = reWhitespace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// --- Progress UI ---

func printProgress(ok, fail, skip, total int, start time.Time) {
	done := ok + fail + skip
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	elapsed := time.Since(start).Round(time.Second)

	// Progress bar: 30 chars wide
	barWidth := 30
	filled := barWidth * done / max(total, 1)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// ETA
	eta := ""
	if done > 0 {
		remaining := time.Duration(float64(elapsed) / float64(done) * float64(total-done))
		eta = fmt.Sprintf(" ETA %s", remaining.Round(time.Second))
	}

	fmt.Printf("\033[2K\r  [%s] %d/%d (%d%%) | \033[32m%d ok\033[0m \033[31m%d fail\033[0m %d skip | %s%s",
		bar, done, total, pct, ok, fail, skip, elapsed, eta)
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// --- Main ---

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime)

	baseURL := strings.TrimRight(*flagURL, "/")
	delay := time.Duration(*flagDelay) * time.Millisecond

	log.Printf("Crawler: %s | limit=%d concurrency=%d delay=%dms",
		baseURL, *flagLimit, *flagConcurrency, *flagDelay)

	// 1. Sitemap → product URLs
	urls, err := fetchProductURLs(baseURL)
	if err != nil {
		log.Fatalf("Sitemap error: %v", err)
	}
	log.Printf("Sitemap: %d URLs", len(urls))

	if len(urls) > *flagLimit {
		urls = urls[:*flagLimit]
	}
	log.Printf("Will crawl %d pages", len(urls))

	// 2. Crawl pages concurrently
	var (
		mu       sync.Mutex
		products []ImportItem
		wg       sync.WaitGroup
		sem      = make(chan struct{}, *flagConcurrency)
		ok, fail, skip int
		total    = len(urls)
		start    = time.Now()
	)

	for i, pageURL := range urls {
		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, url string) {
			defer wg.Done()
			defer func() { <-sem }()

			time.Sleep(delay)

			body, err := fetchURL(url)
			if err != nil {
				mu.Lock()
				fail++
				printProgress(ok, fail, skip, total, start)
				mu.Unlock()
				return
			}

			item := parseProductPage(url, string(body))
			if item == nil {
				mu.Lock()
				skip++
				printProgress(ok, fail, skip, total, start)
				mu.Unlock()
				return
			}

			mu.Lock()
			products = append(products, *item)
			ok++
			// Print product line then progress bar
			cat := item.Category
			if parts := strings.Split(cat, " > "); len(parts) > 0 {
				cat = parts[len(parts)-1]
			}
			if cat == "" {
				cat = "—"
			}
			price := fmt.Sprintf("%d", item.Price/100)
			fmt.Printf("\033[2K  \033[32m✓\033[0m %-50s %6s₽  %s\n", truncate(item.Name, 50), price, cat)
			printProgress(ok, fail, skip, total, start)
			mu.Unlock()
		}(i, pageURL)
	}

	wg.Wait()
	fmt.Print("\033[2K\r") // clear progress line
	elapsed := time.Since(start).Round(time.Second)
	fmt.Printf("\n\033[1mDone:\033[0m %d products, %d failed, %d skipped in %s\n\n", len(products), fail, skip, elapsed)

	if len(products) == 0 {
		log.Fatal("No products extracted")
	}

	// 3. Write JSON to outdir with timestamped filename (never overwrites)
	if err := os.MkdirAll(*flagOutputDir, 0755); err != nil {
		log.Fatalf("Cannot create output dir: %v", err)
	}
	ts := time.Now().Format("2006-01-02_15-04-05")
	outPath := filepath.Join(*flagOutputDir, fmt.Sprintf("crawl_%s_%d.json", ts, len(products)))

	data, err := json.MarshalIndent(ImportRequest{Products: products}, "", "  ")
	if err != nil {
		log.Fatalf("JSON error: %v", err)
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		log.Fatalf("Write error: %v", err)
	}

	log.Printf("Wrote %s (%d products, %.1f KB)", outPath, len(products), float64(len(data))/1024)
}
