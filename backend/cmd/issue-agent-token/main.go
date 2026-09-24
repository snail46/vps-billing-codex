package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"vps-billing/backend/internal/infrastructure/postgres"
	"vps-billing/backend/internal/runman"
)

func main() {
	nodeValue := flag.String("node-id", "", "platform node UUID")
	flag.Parse()
	nodeID, err := uuid.Parse(*nodeValue)
	if err != nil {
		fatal("node-id must be a UUID", err)
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fatal("DATABASE_URL is required", nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		fatal("connect PostgreSQL", err)
	}
	defer database.Close()
	token, err := runman.IssueToken(ctx, database.Pool(), nodeID)
	if err != nil {
		fatal("issue token", err)
	}
	// The token is intentionally shown only once. Only its SHA-256 digest is stored.
	fmt.Println(token)
}

func fatal(message string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	} else {
		fmt.Fprintln(os.Stderr, message)
	}
	os.Exit(1)
}
