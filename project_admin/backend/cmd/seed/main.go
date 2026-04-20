package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type category struct {
	Slug     string
	Name     string
	ParentSlug string // empty for roots
}

var categories = []category{
	// Roots
	{Slug: "face-care", Name: "Face care"},
	{Slug: "makeup", Name: "Makeup"},
	{Slug: "body", Name: "Body"},
	{Slug: "hair", Name: "Hair"},

	// Face care
	{Slug: "cleansing", Name: "Cleansing", ParentSlug: "face-care"},
	{Slug: "toning", Name: "Toning", ParentSlug: "face-care"},
	{Slug: "exfoliation", Name: "Exfoliation", ParentSlug: "face-care"},
	{Slug: "serums", Name: "Serums & ampoules", ParentSlug: "face-care"},
	{Slug: "moisturizing", Name: "Moisturizing", ParentSlug: "face-care"},
	{Slug: "suncare", Name: "Sun care", ParentSlug: "face-care"},
	{Slug: "masks", Name: "Masks", ParentSlug: "face-care"},
	{Slug: "spot-treatment", Name: "Spot treatment", ParentSlug: "face-care"},
	{Slug: "essences", Name: "Essences", ParentSlug: "face-care"},
	{Slug: "lip-care", Name: "Lip care", ParentSlug: "face-care"},

	// Makeup
	{Slug: "makeup-face", Name: "Face", ParentSlug: "makeup"},
	{Slug: "makeup-eyes", Name: "Eyes", ParentSlug: "makeup"},
	{Slug: "makeup-lips", Name: "Lips", ParentSlug: "makeup"},
	{Slug: "makeup-setting", Name: "Setting sprays", ParentSlug: "makeup"},

	// Body
	{Slug: "body-cleansing", Name: "Body cleansing", ParentSlug: "body"},
	{Slug: "body-moisturizing", Name: "Body moisturizing", ParentSlug: "body"},
	{Slug: "body-fragrance", Name: "Fragrance", ParentSlug: "body"},

	// Hair
	{Slug: "hair-shampoo", Name: "Shampoo", ParentSlug: "hair"},
	{Slug: "hair-conditioner", Name: "Conditioner", ParentSlug: "hair"},
	{Slug: "hair-treatment", Name: "Hair treatment", ParentSlug: "hair"},
}

func main() {
	for _, path := range []string{"../../project/.env", ".env"} {
		if err := godotenv.Load(path); err == nil {
			break
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	// Build deterministic UUIDs and slug→id map
	const ns = "6ba7b810-9dad-11d1-80b4-00c04fd430c8" // UUID namespace (DNS)
	nsUUID := uuid.MustParse(ns)
	slugToID := make(map[string]string, len(categories))

	for _, cat := range categories {
		slugToID[cat.Slug] = uuid.NewSHA1(nsUUID, []byte(cat.Slug)).String()
	}

	// Insert categories (roots first, then children)
	inserted := 0
	for _, cat := range categories {
		id := slugToID[cat.Slug]

		var parentID *string
		if cat.ParentSlug != "" {
			pid := slugToID[cat.ParentSlug]
			parentID = &pid
		}

		tag, err := pool.Exec(ctx,
			`INSERT INTO catalog.categories (id, name, slug, parent_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (slug) DO NOTHING`,
			id, cat.Name, cat.Slug, parentID)
		if err != nil {
			log.Fatalf("insert category %q: %v", cat.Slug, err)
		}
		if tag.RowsAffected() > 0 {
			inserted++
		}
	}

	fmt.Printf("Seed complete: %d/%d categories inserted (rest already existed)\n", inserted, len(categories))
}
