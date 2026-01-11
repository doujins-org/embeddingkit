# embeddingkit

`embeddingkit` is a Go library for generating embeddings, storing/searching
vectors in Postgres (pgvector), and running background embedding jobs via a
simple task table.

This README is a **manual** for host applications. For design notes and deeper
details, see `agents/NOTES.md`.

## Host app integration (manual)

### 1) Apply Postgres migrations (required)

embeddingkit migrations are intended to be applied and tracked with migratekit
(`public.migrations`), under `app=embeddingkit`.

This uses migratekit's schema targeting support (via `SET LOCAL search_path =
<host_schema>, public`).

Example (host app):

```go
import (
	"context"
	"database/sql"

	"github.com/doujins-org/embeddingkit/migrations"
	"github.com/doujins-org/migratekit"
)

func applyEmbeddingkitMigrations(ctx context.Context, sqlDB *sql.DB, schema string) error {
	migs, err := migratekit.LoadFromFS(migrations.Postgres)
	if err != nil {
		return err
	}
	m := migratekit.NewPostgres(sqlDB, "embeddingkit").WithSchema(schema)
	if err := m.ApplyMigrations(ctx, migs); err != nil {
		return err
	}
	return m.ValidateAllApplied(ctx, migs)
}
```

### 2) Create embedders (text, and optionally VL)

Use `embedder.NewOpenAICompatible(...)` with your provider’s OpenAI-compatible
base URL + API key + model name.

For VL, the contract is URL-only (the host app provides presigned/public URLs).

### 3) Wire host callbacks

Host apps provide:

- `runtime.DocumentBuilder`: `(entity_type, entity_id) -> text`
- `vl.AssetLister`: `(entity_type, entity_id) -> []assets` (URLs are resolved by the host)

### 4) Enqueue work

When domain entities change (or when you want to backfill), enqueue tasks via:

- `tasks.Repo.Enqueue(ctx, entityType, entityID, model, reason)`

Deduplication is by `(entity_type, entity_id, model)`.

### 5) Run workers

You can run workers with any job runner you want. A minimal loop is:

- `tasks.Repo.FetchReady(...)`
- `runtime.Runtime.GenerateAndStoreEmbedding(...)`
- `tasks.Repo.Complete(...)` / `tasks.Repo.Fail(...)`

Example (non-River):

```go
// package yourapp

import (
	"context"
	"time"

	"github.com/doujins-org/embeddingkit/runtime"
	"github.com/doujins-org/embeddingkit/tasks"
)

func RunEmbeddingWorker(ctx context.Context, rt *runtime.Runtime, repo *tasks.Repo) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			batch, err := repo.FetchReady(ctx, 250, 30*time.Second)
			if err != nil {
				return err
			}
			for _, task := range batch {
				if err := rt.GenerateAndStoreEmbedding(ctx, task.EntityType, task.EntityID, task.Model); err != nil {
					_ = repo.Fail(ctx, task.ID, 30*time.Second)
					continue
				}
				_ = repo.Complete(ctx, task.ID)
			}
		}
	}
}
```

### 6) Query candidates (vector search)

embeddingkit can generate semantic candidates from stored vectors in
`<schema>.embedding_vectors`.

- Query text → candidates: use `search.SearchVectors(...)` with the query vector.
- Similar-to-item → candidates: use `search.SimilarTo(...)` to find neighbors of an existing stored vector.

These APIs return only `(entity_type, entity_id, model, similarity)`; the host
app hydrates those IDs into domain rows and applies business rules.
