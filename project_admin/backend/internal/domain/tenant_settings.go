package domain

type TenantSettings struct {
	Theme           string `json:"theme,omitempty"`
	Currency        string `json:"currency,omitempty"`
	GeoCountry      string `json:"geoCountry,omitempty"`
	GeoRegion       string `json:"geoRegion,omitempty"`
	EnrichCrossData bool   `json:"enrichCrossData,omitempty"`
}
