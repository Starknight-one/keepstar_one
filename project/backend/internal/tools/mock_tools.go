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
and seasonal demand forecasting. Output includes JSON reports with detailed metrics.`,
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
and automated reordering based on inventory thresholds. Includes supplier performance scoring.`,
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
for price sensitivity analysis and revenue optimization algorithms.`,
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
and recommended marketing strategies for each segment.`,
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
Includes integration with external fraud databases and chargeback prediction.`,
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
calculation and sustainable shipping options.`,
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
moderation queue management.`,
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
	}
}

// EstimatedPaddingTokens is approximate token count for padding tools
// 8 tools x ~400 tokens each = ~3200 tokens
// Combined with real tools (~800) = ~4000 tokens, close to 4096 threshold
const EstimatedPaddingTokens = 3200
