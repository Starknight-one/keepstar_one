// Bootstraps the first curator user. Run once after deploying the curator
// service: `go run ./cmd/seed-curator -email vlad@... -password "..."`.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"keepstar-curator/internal/adapters"
)

func main() {
	email := flag.String("email", "", "curator email")
	password := flag.String("password", "", "curator password (min 12 chars)")
	flag.Parse()
	if *email == "" || len(*password) < 12 {
		log.Fatal("usage: seed-curator -email X -password Y (password >= 12 chars)")
	}
	dsn := firstNonEmpty(os.Getenv("CURATOR_DATABASE_URL"), os.Getenv("DATABASE_URL"))
	if dsn == "" {
		log.Fatal("CURATOR_DATABASE_URL or DATABASE_URL required")
	}
	ctx := context.Background()
	client, err := adapters.NewClient(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer client.Close()
	if err := client.RunMigrations(ctx); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	id, err := client.CreateUser(ctx, *email, *password)
	if err != nil {
		log.Fatalf("create user: %v", err)
	}
	fmt.Printf("created curator user %s (%s)\n", id, *email)
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
