package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/family-habit/family-habit/backend/internal/database"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "status") {
		log.Fatal("usage: migrate up|status")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if os.Args[1] == "up" {
		if err := database.MigrateUp(ctx, pool); err != nil {
			log.Fatal(err)
		}
		fmt.Println("migrations applied")
		return
	}
	versions, err := database.MigrationStatus(ctx, pool)
	if err != nil {
		log.Fatal(err)
	}
	for _, version := range versions {
		fmt.Println(version)
	}
}
