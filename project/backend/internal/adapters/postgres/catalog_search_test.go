package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"keepstar/internal/adapters/postgres"
	"keepstar/internal/ports"
)

func TestCatalogSearch(t *testing.T) {
	_ = godotenv.Load("../.env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbClient, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("DB connection failed: %v", err)
	}
	defer dbClient.Close()

	catalogAdapter := postgres.NewCatalogAdapter(dbClient)

	// Get Nike tenant
	tenant, err := catalogAdapter.GetTenantBySlug(ctx, "nike")
	if err != nil {
		t.Fatalf("Error getting tenant: %v", err)
	}
	fmt.Printf("Tenant: %s (ID: %s)\n\n", tenant.Name, tenant.ID)

	// Test 1: No filters - should return all products
	fmt.Println("=== Test 1: No filters ===")
	products, total, err := catalogAdapter.ListProducts(ctx, tenant.ID, ports.ProductFilter{
		Limit: 10,
	})
	if err != nil {
		t.Errorf("Error: %v", err)
	} else {
		fmt.Printf("Found: %d products\n", total)
		for _, p := range products {
			fmt.Printf("  - %s (brand: %s, price: %d)\n", p.Name, p.Brand, p.Price)
		}
	}

	// Test 2: Brand filter only
	fmt.Println("\n=== Test 2: Brand filter only ===")
	products, total, err = catalogAdapter.ListProducts(ctx, tenant.ID, ports.ProductFilter{
		Brand: "Nike",
		Limit: 5,
	})
	if err != nil {
		t.Errorf("Error: %v", err)
	} else {
		fmt.Printf("Found: %d products\n", total)
		for _, p := range products {
			fmt.Printf("  - %s (brand: %s)\n", p.Name, p.Brand)
		}
	}

	// Test 3: Search query
	fmt.Println("\n=== Test 3: Search 'Air Max' ===")
	products, total, err = catalogAdapter.ListProducts(ctx, tenant.ID, ports.ProductFilter{
		Search: "Air Max",
		Limit:  5,
	})
	if err != nil {
		t.Errorf("Error: %v", err)
	} else {
		fmt.Printf("Found: %d products\n", total)
		for _, p := range products {
			fmt.Printf("  - %s\n", p.Name)
		}
	}
}
