package domain

import (
	"fmt"
	"strings"
	"time"
)

// CatalogDigest is a pre-computed meta-schema of a tenant's catalog.
// Compact format (~300-400 tokens) — sent once at session init, cached by Anthropic.
type CatalogDigest struct {
	GeneratedAt    time.Time               `json:"generated_at"`
	TotalProducts  int                     `json:"total_products"`
	CategoryTree   []DigestCategoryGroup   `json:"category_tree"`
	SharedFilters  []DigestSharedFilter    `json:"shared_filters"`
	TopBrands      []string                `json:"top_brands,omitempty"`
	TopIngredients []string                `json:"top_ingredients,omitempty"`
}

// DigestCategoryGroup is a root category with leaf children.
type DigestCategoryGroup struct {
	Name     string                `json:"name"`
	Slug     string                `json:"slug"`
	Children []DigestCategoryLeaf  `json:"children"`
}

// DigestCategoryLeaf is a leaf category with product count.
type DigestCategoryLeaf struct {
	Name  string   `json:"name"`
	Slug  string   `json:"slug"`
	Count int      `json:"count"`
	Forms []string `json:"forms,omitempty"` // product_form values specific to this category
}

// DigestSharedFilter is a global filter with all possible values.
type DigestSharedFilter struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

// ToPromptText returns ultra-compact text for LLM context (~300-400 tokens).
func (d *CatalogDigest) ToPromptText() string {
	if d == nil || d.TotalProducts == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d products\n\n", d.TotalProducts))

	// Category tree: compact "parent: child(N), child(N), ..."
	b.WriteString("categories:\n")
	for _, group := range d.CategoryTree {
		if len(group.Children) == 0 {
			continue
		}
		b.WriteString("  ")
		b.WriteString(group.Slug)
		b.WriteString(": ")
		for i, child := range group.Children {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf("%s(%d)", child.Slug, child.Count))
		}
		b.WriteString("\n")
	}

	// Shared filters
	if len(d.SharedFilters) > 0 {
		b.WriteString("\nfilters:\n")
		for _, f := range d.SharedFilters {
			b.WriteString(fmt.Sprintf("  %s: %s\n", f.Key, strings.Join(f.Values, "|")))
		}
	}

	// Top brands
	if len(d.TopBrands) > 0 {
		b.WriteString(fmt.Sprintf("\nbrands(%d): %s\n", len(d.TopBrands), strings.Join(d.TopBrands, ", ")))
	}

	// Top ingredients
	if len(d.TopIngredients) > 0 {
		b.WriteString(fmt.Sprintf("ingredients(%d): %s\n", len(d.TopIngredients), strings.Join(d.TopIngredients, ", ")))
	}

	// One-line search rule
	b.WriteString("\nenum value → filters.{key}, free text → vector_query\n")

	return b.String()
}
