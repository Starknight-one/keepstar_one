package engine

import (
	"keepstar/internal/domain"
)

// FieldTypeEntry maps a field name to its AtomType and Subtype for dynamic field construction
type FieldTypeEntry struct {
	Type    domain.AtomType
	Subtype domain.AtomSubtype
}

// FieldTypeMap resolves field name -> AtomType/Subtype for fields[] construction
var FieldTypeMap = map[string]FieldTypeEntry{
	"name":          {domain.AtomTypeText, domain.SubtypeString},
	"description":   {domain.AtomTypeText, domain.SubtypeString},
	"brand":         {domain.AtomTypeText, domain.SubtypeString},
	"category":      {domain.AtomTypeText, domain.SubtypeString},
	"price":         {domain.AtomTypeNumber, domain.SubtypeCurrency},
	"rating":        {domain.AtomTypeNumber, domain.SubtypeRating},
	"images":        {domain.AtomTypeImage, domain.SubtypeImageURL},
	"stockQuantity": {domain.AtomTypeNumber, domain.SubtypeInt},
	"tags":          {domain.AtomTypeText, domain.SubtypeString},
	"attributes":    {domain.AtomTypeText, domain.SubtypeString},
	"duration":       {domain.AtomTypeText, domain.SubtypeString},
	"provider":       {domain.AtomTypeText, domain.SubtypeString},
	"availability":   {domain.AtomTypeText, domain.SubtypeString},
	"productForm":    {domain.AtomTypeText, domain.SubtypeString},
	"skinType":       {domain.AtomTypeText, domain.SubtypeString},
	"concern":        {domain.AtomTypeText, domain.SubtypeString},
	"keyIngredients": {domain.AtomTypeText, domain.SubtypeString},
}
