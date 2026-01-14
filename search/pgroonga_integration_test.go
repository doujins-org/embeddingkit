package search

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration test.
//
// Run with a PGroonga-enabled Postgres and set:
//
//	SEARCHKIT_PGROONGA_URL=postgres://postgres:pass@localhost:55432/testdb?sslmode=disable
//
// Example docker:
//
//	docker run --rm -d --name sk_pgroonga -p 55432:5432 -e POSTGRES_PASSWORD=pass -e POSTGRES_DB=testdb groonga/pgroonga:latest
//	SEARCHKIT_PGROONGA_URL=postgres://postgres:pass@localhost:55432/testdb?sslmode=disable go test ./... -run PGroonga
func TestPGroongaSearch_Integration_JA(t *testing.T) {
	dsn := os.Getenv("SEARCHKIT_PGROONGA_URL")
	if dsn == "" {
		t.Skip("SEARCHKIT_PGROONGA_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()

	// Minimal schema setup for lexical + PGroonga only.
	_, err = pool.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS s;
		SET search_path = s, public;
		CREATE EXTENSION IF NOT EXISTS pgroonga;
		CREATE EXTENSION IF NOT EXISTS pg_trgm;
		CREATE TABLE IF NOT EXISTS search_documents (
			entity_type text NOT NULL,
			entity_id text NOT NULL,
			language text NOT NULL,
			raw_document text,
			document text NOT NULL,
			tsv tsvector,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (entity_type, entity_id, language)
		);
		CREATE OR REPLACE FUNCTION searchkit_regconfig_for_language(lang text)
		RETURNS regconfig
		LANGUAGE sql
		IMMUTABLE
		AS $$
			SELECT 'simple'::regconfig
		$$;
		CREATE INDEX IF NOT EXISTS idx_search_documents_raw_document_pgroonga_cjk
			ON search_documents USING pgroonga (raw_document)
			WHERE language IN ('ja','zh','ko');
		TRUNCATE TABLE search_documents;
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Insert a Japanese title with no spaces (requires segmentation).
	_, err = pool.Exec(ctx, `
		INSERT INTO s.search_documents(entity_type, entity_id, language, raw_document, document, tsv)
		VALUES ('series', '1', 'ja', '鬼滅の刃', 'kimetsu no yaiba', to_tsvector(searchkit_regconfig_for_language('ja'), '鬼滅の刃'))
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	hits, err := PGroongaSearch(ctx, pool, "鬼滅", PGroongaOptions{
		Schema:   "s",
		Language: "ja",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("PGroongaSearch: %v", err)
	}
	if len(hits) == 0 || hits[0].EntityID != "1" {
		t.Fatalf("expected hit entity_id=1, got %+v", hits)
	}

	prefixHits, err := PGroongaSearch(ctx, pool, "鬼", PGroongaOptions{
		Schema:   "s",
		Language: "ja",
		Limit:    10,
		Prefix:   true,
	})
	if err != nil {
		t.Fatalf("PGroongaSearch prefix: %v", err)
	}
	if len(prefixHits) == 0 || prefixHits[0].EntityID != "1" {
		t.Fatalf("expected prefix hit entity_id=1, got %+v", prefixHits)
	}
}
