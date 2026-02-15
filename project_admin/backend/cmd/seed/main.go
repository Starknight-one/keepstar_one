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
	{Slug: "face-care", Name: "Уход за лицом"},
	{Slug: "makeup", Name: "Декоративная косметика"},
	{Slug: "body", Name: "Тело"},
	{Slug: "hair", Name: "Волосы"},

	// Face care
	{Slug: "cleansing", Name: "Очищение", ParentSlug: "face-care"},
	{Slug: "toning", Name: "Тонизирование", ParentSlug: "face-care"},
	{Slug: "exfoliation", Name: "Эксфолиация", ParentSlug: "face-care"},
	{Slug: "serums", Name: "Сыворотки и ампулы", ParentSlug: "face-care"},
	{Slug: "moisturizing", Name: "Увлажнение", ParentSlug: "face-care"},
	{Slug: "suncare", Name: "Солнцезащита", ParentSlug: "face-care"},
	{Slug: "masks", Name: "Маски", ParentSlug: "face-care"},
	{Slug: "spot-treatment", Name: "Точечные средства", ParentSlug: "face-care"},
	{Slug: "essences", Name: "Эссенции", ParentSlug: "face-care"},
	{Slug: "lip-care", Name: "Уход за губами", ParentSlug: "face-care"},

	// Makeup
	{Slug: "makeup-face", Name: "Для лица", ParentSlug: "makeup"},
	{Slug: "makeup-eyes", Name: "Для глаз", ParentSlug: "makeup"},
	{Slug: "makeup-lips", Name: "Для губ", ParentSlug: "makeup"},
	{Slug: "makeup-setting", Name: "Фиксаторы макияжа", ParentSlug: "makeup"},

	// Body
	{Slug: "body-cleansing", Name: "Очищение тела", ParentSlug: "body"},
	{Slug: "body-moisturizing", Name: "Увлажнение тела", ParentSlug: "body"},
	{Slug: "body-fragrance", Name: "Парфюм", ParentSlug: "body"},

	// Hair
	{Slug: "hair-shampoo", Name: "Шампуни", ParentSlug: "hair"},
	{Slug: "hair-conditioner", Name: "Кондиционеры", ParentSlug: "hair"},
	{Slug: "hair-treatment", Name: "Уход за волосами", ParentSlug: "hair"},
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
