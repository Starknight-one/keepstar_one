package domain

import "time"

// Plan is one subscription tier. The catalog is hardcoded (see PlanCatalog)
// because plans rarely change and we don't want a plans table until there
// is a reason to edit them from the admin UI.
type Plan struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	TokensPerMonth int64  `json:"tokensPerMonth"`
	Conversations  string `json:"conversations"`
	Products       string `json:"products"`
	Seats          string `json:"seats"`
	Support        string `json:"support"`
}

// Subscription is the per-tenant row in admin.billing_subscriptions.
// Token multiplier lives here so we can later vary it per tenant without
// touching the pricing display logic in the frontend.
type Subscription struct {
	TenantID         string    `json:"tenantId"`
	Plan             string    `json:"plan"`
	Status           string    `json:"status"`
	CycleStart       time.Time `json:"cycleStart"`
	CycleEnd         time.Time `json:"cycleEnd"`
	TokenMultiplier  float64   `json:"tokenMultiplier"`
	AlertThreshold   int       `json:"alertThreshold"`
	AlertSlack       bool      `json:"alertSlack"`
	OverdraftEnabled bool      `json:"overdraftEnabled"`
	OverdraftNotify  bool      `json:"overdraftNotify"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// Preferences is the writeable slice of Subscription exposed on
// POST /billing/preferences.
type Preferences struct {
	AlertThreshold   int  `json:"alertThreshold"`
	AlertSlack       bool `json:"alertSlack"`
	OverdraftEnabled bool `json:"overdraftEnabled"`
	OverdraftNotify  bool `json:"overdraftNotify"`
}

// Invoice is one row in admin.billing_invoices. TokensUsed is already
// multiplier-adjusted at insert time so list pages don't need to know the
// multiplier. AmountUSD is nil for upcoming invoices.
type Invoice struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenantId"`
	PeriodStart time.Time  `json:"periodStart"`
	PeriodEnd   time.Time  `json:"periodEnd"`
	TokensUsed  int64      `json:"tokensUsed"`
	AmountUSD   *float64   `json:"amountUsd"`
	Status      string     `json:"status"`
	PDFURL      *string    `json:"pdfUrl"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// BillingOverview is the composed /billing/overview response.
type BillingOverview struct {
	Plan           Plan           `json:"plan"`
	Subscription   BillingSubView `json:"subscription"`
	Usage          BillingUsage   `json:"usage"`
	Preferences    Preferences    `json:"preferences"`
	AvailablePlans []Plan         `json:"availablePlans"`
}

type BillingSubView struct {
	Status     string    `json:"status"`
	CycleStart time.Time `json:"cycleStart"`
	CycleEnd   time.Time `json:"cycleEnd"`
	RenewsAt   time.Time `json:"renewsAt"`
}

type BillingUsage struct {
	TokensUsed       int64   `json:"tokensUsed"`
	TokensQuota      int64   `json:"tokensQuota"`
	APITokensActual  int64   `json:"apiTokensActual"`
	Multiplier       float64 `json:"multiplier"`
	CostUSDActual    float64 `json:"costUsdActual"`
}

// PlanCatalog is the single source of truth for plan metadata.
//
// Copy is deliberately English-only (user-facing text rule).
var PlanCatalog = map[string]Plan{
	"starter": {
		ID:             "starter",
		Name:           "Starter",
		TokensPerMonth: 10000,
		Conversations:  "1,000 / mo",
		Products:       "Up to 1,000",
		Seats:          "3 included",
		Support:        "Standard",
	},
	"growth": {
		ID:             "growth",
		Name:           "Growth",
		TokensPerMonth: 50000,
		Conversations:  "5,000 / mo",
		Products:       "Up to 10,000",
		Seats:          "10 included",
		Support:        "Priority · 24h SLA",
	},
	"enterprise": {
		ID:             "enterprise",
		Name:           "Enterprise",
		TokensPerMonth: 200000,
		Conversations:  "Unlimited",
		Products:       "Unlimited",
		Seats:          "Unlimited",
		Support:        "Dedicated CSM",
	},
}

// PlanOrder is the canonical order used wherever we render the list of
// plans — low-to-high so Upgrade / Downgrade copy stays consistent.
var PlanOrder = []string{"starter", "growth", "enterprise"}

// IsValidPlan gates POST /billing/plan.
func IsValidPlan(id string) bool {
	_, ok := PlanCatalog[id]
	return ok
}
