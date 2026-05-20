package domain

type TenantSettings struct {
	Theme           string `json:"theme,omitempty"`
	Currency        string `json:"currency,omitempty"`
	GeoCountry      string `json:"geoCountry,omitempty"`
	GeoRegion       string `json:"geoRegion,omitempty"`
	EnrichCrossData bool   `json:"enrichCrossData,omitempty"`
	WidgetEnabled   bool   `json:"widgetEnabled"`
	// DailySyncEnabled — when true, the background cron picks this tenant
	// up once every 24h and re-applies their inbox. Default off; meant for
	// Shopify-installed merchants who want a daily safety net on top of
	// the webhook subscription. Stored as settings->>'daily_sync_enabled'
	// on catalog.tenants; the cron filter SQL reads that key directly.
	DailySyncEnabled bool `json:"dailySyncEnabled"`
}
