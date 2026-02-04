package tools

import "keepstar/internal/domain"

// CachePaddingEnabled controls whether padding tools are added
// TODO: Remove when real tools exceed 4096 tokens
var CachePaddingEnabled = true

// GetCachePaddingTools returns dummy tools to reach cache threshold
// These tools have detailed descriptions but will never be called
// because their names start with _internal_ prefix
func GetCachePaddingTools() []domain.ToolDefinition {
	if !CachePaddingEnabled {
		return nil
	}

	return []domain.ToolDefinition{
		{
			Name: "_internal_inventory_analytics",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool generates comprehensive inventory analytics reports for administrative dashboards.
It processes warehouse data, calculates stock levels, predicts reorder points, and generates
trend analysis for inventory management. Supports multiple warehouse locations, SKU tracking,
and seasonal demand forecasting. Output includes JSON reports with detailed metrics.
Advanced features include safety stock calculation using statistical models, ABC analysis
for inventory classification, dead stock identification with configurable thresholds,
and cross-warehouse transfer optimization. The system monitors stock-to-sales ratios,
inventory turnover rates, and carrying cost projections across all product categories.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"warehouse_id":     map[string]interface{}{"type": "string", "description": "Internal warehouse identifier"},
					"date_range":       map[string]interface{}{"type": "string", "description": "Analysis period"},
					"include_forecast": map[string]interface{}{"type": "boolean", "description": "Include demand forecast"},
				},
			},
		},
		{
			Name: "_internal_supplier_integration",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool manages supplier integration workflows for the procurement system.
It handles purchase order generation, supplier communication protocols, delivery tracking,
and invoice reconciliation. Supports EDI formats, API integrations with major suppliers,
and automated reordering based on inventory thresholds. Includes supplier performance scoring.
Extended capabilities include multi-tier supplier network management, contract compliance
monitoring, lead time variability tracking, and vendor-managed inventory programs.
The system also handles dropship routing, split shipment coordination, and automated
quality assurance checkpoints at each stage of the procurement pipeline.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"supplier_id": map[string]interface{}{"type": "string", "description": "Supplier identifier"},
					"action":      map[string]interface{}{"type": "string", "description": "Integration action type"},
				},
			},
		},
		{
			Name: "_internal_pricing_engine",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool operates the dynamic pricing engine for competitive price optimization.
It analyzes competitor prices, demand elasticity, inventory levels, and margin targets
to suggest optimal pricing strategies. Supports A/B testing of price points, promotional
pricing rules, and geographic price differentiation. Includes machine learning models
for price sensitivity analysis and revenue optimization algorithms. Additional modules
cover bundle pricing optimization, volume discount tier management, flash sale scheduling,
loyalty member price overrides, and marketplace fee-aware margin calculation.
The engine maintains real-time competitor price feeds and adjusts recommendations
based on time-of-day demand patterns and seasonal trend coefficients.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"product_ids": map[string]interface{}{"type": "array", "description": "Products to analyze"},
					"strategy":    map[string]interface{}{"type": "string", "description": "Pricing strategy"},
				},
			},
		},
		{
			Name: "_internal_customer_segmentation",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool performs advanced customer segmentation for marketing analytics.
It clusters customers based on purchase history, browsing behavior, demographics,
and engagement metrics. Supports RFM analysis, cohort analysis, lifetime value prediction,
and churn risk scoring. Output includes segment definitions, customer assignments,
and recommended marketing strategies for each segment. The segmentation engine also
provides lookalike audience modeling, cross-sell propensity scoring, win-back campaign
targeting, and VIP customer identification. Behavioral clustering uses session depth,
page dwell time, cart abandonment patterns, and wishlist activity as input features.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"segment_type":            map[string]interface{}{"type": "string", "description": "Segmentation method"},
					"include_recommendations": map[string]interface{}{"type": "boolean", "description": "Include marketing recs"},
				},
			},
		},
		{
			Name: "_internal_fraud_detection",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool runs fraud detection algorithms on transaction data.
It analyzes payment patterns, device fingerprints, geographic anomalies,
and behavioral signals to identify potentially fraudulent orders. Supports
real-time scoring, rule-based detection, and machine learning models.
Includes integration with external fraud databases and chargeback prediction.
The detection pipeline incorporates velocity checks, BIN-country mismatch alerts,
proxy and VPN detection, and social graph analysis for organized fraud rings.
Risk scores are computed using gradient-boosted decision trees trained on
historical chargeback data with automatic model retraining on a weekly cadence.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"transaction_id": map[string]interface{}{"type": "string", "description": "Transaction to analyze"},
					"check_type":     map[string]interface{}{"type": "string", "description": "Type of fraud check"},
				},
			},
		},
		{
			Name: "_internal_shipping_optimizer",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool optimizes shipping routes and carrier selection for order fulfillment.
It calculates optimal shipping methods based on package dimensions, weight, destination,
delivery speed requirements, and cost constraints. Supports multi-carrier rate shopping,
zone skipping strategies, and consolidation opportunities. Includes carbon footprint
calculation and sustainable shipping options. The optimizer also handles last-mile delivery
scheduling, pickup point network allocation, international customs documentation
generation, and duty/tax pre-calculation for cross-border shipments. Route planning
uses graph-based algorithms with real-time traffic and weather data integration.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"order_ids":    map[string]interface{}{"type": "array", "description": "Orders to optimize"},
					"optimize_for": map[string]interface{}{"type": "string", "description": "cost/speed/carbon"},
				},
			},
		},
		{
			Name: "_internal_content_moderation",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool performs content moderation on user-generated content including reviews,
questions, and uploaded images. It detects inappropriate content, spam, fake reviews,
and policy violations using NLP and computer vision models. Supports multiple languages,
sentiment analysis, and automated flagging workflows. Includes appeal handling and
moderation queue management. Extended capabilities cover automated SEO spam detection,
competitor mention filtering, PII redaction in user submissions, and brand safety
scoring for marketplace seller content. The vision module identifies counterfeit
product images, watermark violations, and misleading photo editing in listings.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content_id":   map[string]interface{}{"type": "string", "description": "Content to moderate"},
					"content_type": map[string]interface{}{"type": "string", "description": "review/question/image"},
				},
			},
		},
		{
			Name: "_internal_recommendation_engine",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool generates personalized product recommendations using collaborative filtering
and content-based algorithms. It analyzes user behavior, purchase history, product attributes,
and real-time context to suggest relevant products. Supports multiple recommendation types:
similar products, frequently bought together, personalized picks, and trending items.
Includes A/B testing framework and performance analytics.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"user_id": map[string]interface{}{"type": "string", "description": "User for recommendations"},
					"context": map[string]interface{}{"type": "string", "description": "Recommendation context"},
					"limit":   map[string]interface{}{"type": "integer", "description": "Number of recommendations"},
				},
			},
		},
		{
			Name: "_internal_loyalty_program",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool manages the customer loyalty and rewards program for the platform.
It tracks points accumulation from purchases, referrals, and engagement activities.
Supports tier-based membership levels (bronze, silver, gold, platinum) with escalating
benefits and discount multipliers. Handles reward redemption, points expiration policies,
birthday bonuses, and partner cross-promotion campaigns. Includes analytics for program
effectiveness measurement and ROI tracking across customer segments.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"customer_id":   map[string]interface{}{"type": "string", "description": "Customer identifier for loyalty lookup"},
					"action":        map[string]interface{}{"type": "string", "description": "earn/redeem/status/history"},
					"points_amount": map[string]interface{}{"type": "integer", "description": "Points to earn or redeem"},
					"campaign_id":   map[string]interface{}{"type": "string", "description": "Associated marketing campaign"},
				},
			},
		},
		{
			Name: "_internal_returns_processor",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool processes product returns and manages the reverse logistics workflow.
It handles return authorization requests, validates return eligibility windows,
calculates refund amounts including partial refunds and restocking fees.
Supports multiple return reasons (defective, wrong item, changed mind, not as described),
automated quality inspection routing, and warehouse receiving workflows.
Includes integration with payment processors for refund disbursement and
carrier label generation for prepaid return shipping.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"order_id":      map[string]interface{}{"type": "string", "description": "Original order identifier"},
					"item_ids":      map[string]interface{}{"type": "array", "description": "Items to return from the order"},
					"return_reason": map[string]interface{}{"type": "string", "description": "Reason category for the return"},
					"refund_method": map[string]interface{}{"type": "string", "description": "original_payment/store_credit/exchange"},
				},
			},
		},
		{
			Name: "_internal_ab_testing_framework",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool manages A/B testing and experimentation infrastructure for the platform.
It handles experiment creation, traffic allocation, variant assignment, and statistical
significance calculation. Supports multivariate testing, feature flags, gradual rollouts,
and holdout groups. The framework provides Bayesian and frequentist analysis methods,
sequential testing with optional stopping rules, and sample ratio mismatch detection.
Includes integration with analytics pipelines for metric computation and alerting
on guardrail metric regressions during active experiments.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"experiment_id": map[string]interface{}{"type": "string", "description": "Experiment identifier"},
					"action":        map[string]interface{}{"type": "string", "description": "create/assign/analyze/conclude"},
					"variants":      map[string]interface{}{"type": "array", "description": "Variant configurations"},
				},
			},
		},
		{
			Name: "_internal_tax_compliance",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool handles tax calculation, compliance, and reporting across multiple jurisdictions.
It computes sales tax, VAT, GST, and digital services tax based on buyer and seller locations.
Supports nexus determination, tax exemption certificate management, and marketplace facilitator
rules. The engine maintains up-to-date tax rate databases for all supported countries and
subdivisions. Includes automated filing preparation, tax remittance scheduling, and audit
trail generation for regulatory compliance requirements across domestic and cross-border
transactions.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"transaction_id": map[string]interface{}{"type": "string", "description": "Transaction for tax calculation"},
					"jurisdiction":   map[string]interface{}{"type": "string", "description": "Tax jurisdiction code"},
					"action":         map[string]interface{}{"type": "string", "description": "calculate/exempt/report/file"},
				},
			},
		},
		{
			Name: "_internal_warehouse_management",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool operates the warehouse management system for inventory storage and order picking.
It manages bin locations, pick paths, pack stations, and shipping dock assignments.
Supports wave picking, batch picking, and zone picking strategies with dynamic optimization
based on order volume and SKU velocity. The system handles receiving workflows, putaway
logic, cycle counting, and physical inventory reconciliation. Includes slotting optimization
algorithms that minimize travel time and labor cost per pick across multi-level racking
systems and automated storage and retrieval configurations.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"warehouse_id": map[string]interface{}{"type": "string", "description": "Warehouse facility identifier"},
					"operation":    map[string]interface{}{"type": "string", "description": "receive/putaway/pick/pack/ship"},
					"order_ids":    map[string]interface{}{"type": "array", "description": "Orders to process"},
				},
			},
		},
		{
			Name: "_internal_email_campaign",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool manages email marketing campaigns and transactional email delivery.
It handles template rendering, audience segmentation, send scheduling, and deliverability
optimization. Supports drip campaigns, triggered emails, cart abandonment sequences,
and win-back flows. The system tracks open rates, click-through rates, conversion attribution,
and unsubscribe management. Includes spam score pre-checking, inbox placement prediction,
send time optimization per recipient timezone, and automated subject line A/B testing
with multi-armed bandit allocation for maximum engagement.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"campaign_id": map[string]interface{}{"type": "string", "description": "Campaign identifier"},
					"action":      map[string]interface{}{"type": "string", "description": "create/schedule/send/analyze"},
					"audience_id": map[string]interface{}{"type": "string", "description": "Target audience segment"},
				},
			},
		},
		{
			Name: "_internal_search_relevance",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool manages search relevance tuning and query understanding for the product catalog.
It handles synonym management, query expansion, spelling correction, and intent classification.
Supports learning-to-rank models with features including BM25 scores, click-through rates,
conversion rates, and freshness signals. The system provides search analytics including
zero-result queries, query refinement patterns, and search abandonment tracking.
Includes faceted search configuration, typeahead suggestion ranking, and personalized
search result boosting based on user preference profiles and browsing history vectors.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action":     map[string]interface{}{"type": "string", "description": "tune/analyze/synonyms/boost"},
					"query":      map[string]interface{}{"type": "string", "description": "Search query to analyze"},
					"model_id":   map[string]interface{}{"type": "string", "description": "Ranking model identifier"},
				},
			},
		},
		{
			Name: "_internal_payment_orchestration",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool orchestrates payment processing across multiple payment service providers.
It handles payment method selection, 3DS authentication flows, tokenization, and failover
routing between processors. Supports credit cards, digital wallets, bank transfers,
buy-now-pay-later integrations, and cryptocurrency payments. The system manages payment
splitting for marketplace transactions, escrow holds, and scheduled disbursements.
Includes PCI DSS compliance tooling, payment fraud screening integration, chargeback
management workflows, and reconciliation between processor settlements and internal
ledger entries across multiple currencies and settlement accounts.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"payment_id":  map[string]interface{}{"type": "string", "description": "Payment transaction identifier"},
					"action":      map[string]interface{}{"type": "string", "description": "authorize/capture/refund/void"},
					"provider":    map[string]interface{}{"type": "string", "description": "Payment provider preference"},
					"amount":      map[string]interface{}{"type": "number", "description": "Payment amount in minor units"},
				},
			},
		},
		{
			Name: "_internal_notification_hub",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool manages multi-channel notification delivery including push notifications,
SMS, in-app messages, and webhook callbacks. It handles notification template management,
channel preference routing, delivery scheduling, and quiet hours enforcement per user
timezone. Supports notification batching, frequency capping, priority escalation, and
delivery receipt tracking. The system provides real-time delivery analytics, channel
performance comparison, and opt-in/opt-out preference management across all touchpoints.
Includes rich notification formatting with deep linking, action buttons, and inline
media attachments for supported platforms.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"recipient_id":  map[string]interface{}{"type": "string", "description": "User to notify"},
					"channel":       map[string]interface{}{"type": "string", "description": "push/sms/in_app/webhook"},
					"template_id":   map[string]interface{}{"type": "string", "description": "Notification template"},
					"priority":      map[string]interface{}{"type": "string", "description": "low/normal/high/critical"},
				},
			},
		},
		{
			Name: "_internal_catalog_enrichment",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool performs automated product catalog enrichment and data quality management.
It extracts product attributes from unstructured descriptions using NLP models, normalizes
size and color values across brands, and generates SEO-optimized titles and meta descriptions.
Supports image background removal, automated alt-text generation, and product taxonomy
classification. The system validates mandatory attribute completeness, detects duplicate
listings, and identifies data inconsistencies across marketplace seller feeds. Includes
bulk import validation, schema migration tooling, and attribute inheritance rules for
product variant hierarchies.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"product_ids":  map[string]interface{}{"type": "array", "description": "Products to enrich"},
					"enrichment":   map[string]interface{}{"type": "string", "description": "attributes/images/seo/taxonomy"},
					"source":       map[string]interface{}{"type": "string", "description": "Data source for enrichment"},
				},
			},
		},
		{
			Name: "_internal_analytics_pipeline",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool manages the real-time and batch analytics data pipeline for business intelligence.
It handles event ingestion, schema validation, deduplication, and routing to downstream
consumers. Supports clickstream processing, conversion funnel computation, revenue attribution
modeling, and cohort retention analysis. The pipeline includes data quality monitoring with
anomaly detection, late-arriving event handling, and exactly-once processing guarantees.
Provides pre-built dashboard widgets for GMV tracking, active user metrics, category
performance, and seller scorecards with configurable aggregation windows and drill-down
dimensions.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pipeline_id":  map[string]interface{}{"type": "string", "description": "Analytics pipeline identifier"},
					"action":       map[string]interface{}{"type": "string", "description": "ingest/process/query/dashboard"},
					"time_range":   map[string]interface{}{"type": "string", "description": "Analysis time window"},
				},
			},
		},
		{
			Name: "_internal_compliance_audit",
			Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool manages regulatory compliance auditing and data governance across the platform.
It tracks data processing activities for GDPR, CCPA, and other privacy regulations.
Supports data subject access requests, right-to-erasure workflows, consent management,
and data retention policy enforcement. The system maintains immutable audit logs for all
data access events, generates compliance reports for regulatory submissions, and monitors
third-party data processor agreements. Includes automated data classification, sensitive
data discovery scanning, and cross-border data transfer impact assessments with standard
contractual clause tracking and adequacy decision monitoring.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"audit_type":   map[string]interface{}{"type": "string", "description": "gdpr/ccpa/pci/sox compliance type"},
					"action":       map[string]interface{}{"type": "string", "description": "scan/report/remediate/certify"},
					"scope":        map[string]interface{}{"type": "string", "description": "Audit scope definition"},
				},
			},
		},
	}
}

// EstimatedPaddingTokens is approximate token count for padding tools
// 20 tools x ~200 tokens each = ~4000 tokens
// Combined with real tools (~800) + system prompt (~500) = ~5300 tokens, safely above 4096 threshold
const EstimatedPaddingTokens = 4000
