// decrypt-shopify-token — pulls the encrypted Shopify offline access token from
// admin.tenant_integrations and prints the plaintext. Used to feed cmd/seed-devstore
// without doing a fresh OAuth dance.
//
// Usage:
//   ADMIN_ENCRYPTION_KEY=<base64> DATABASE_URL=... \
//   go run ./cmd/decrypt-shopify-token -shop keepstar-neaqpan1.myshopify.com
//
// Output: shop domain on stderr, plaintext token on stdout (so it can be
// piped: SHOPIFY_TOKEN=$(go run ... -shop X) go run ./cmd/seed-devstore).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"keepstar-admin/internal/crypto/secretbox"
)

func main() {
	shop := flag.String("shop", "", "shop domain (e.g. keepstar-neaqpan1.myshopify.com)")
	flag.Parse()
	if *shop == "" {
		log.Fatal("usage: decrypt-shopify-token -shop <shop>.myshopify.com")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL required")
	}
	keyB64 := os.Getenv("ADMIN_ENCRYPTION_KEY")
	if keyB64 == "" {
		log.Fatal("ADMIN_ENCRYPTION_KEY required (base64 of 32 random bytes — same as prod admin)")
	}
	box, err := secretbox.NewFromBase64(keyB64)
	if err != nil {
		log.Fatalf("decrypt key: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	var encrypted, status string
	err = pool.QueryRow(ctx, `
		SELECT credentials_encrypted, COALESCE(status, '')
		FROM admin.tenant_integrations
		WHERE kind = 'shopify' AND external_id = $1
		ORDER BY created_at DESC LIMIT 1`, *shop).Scan(&encrypted, &status)
	if err != nil {
		log.Fatalf("lookup: %v", err)
	}
	plaintext, err := box.Open(encrypted)
	if err != nil {
		log.Fatalf("decrypt: %v (wrong ADMIN_ENCRYPTION_KEY?)", err)
	}
	fmt.Fprintf(os.Stderr, "shop=%s status=%s\n", *shop, status)
	fmt.Print(string(plaintext))
}
