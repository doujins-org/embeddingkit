package searchkit

import (
	"context"
	"strings"

	"github.com/doujins-org/searchkit/search"
	"github.com/jackc/pgx/v5/pgxpool"
)

func isCJKLanguage(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "ja", "zh", "ko":
		return true
	default:
		return false
	}
}

// Typeahead is the recommended entrypoint for trigram-based suggestions while typing.
//
// Under the hood it uses:
//   - `pg_trgm` over `<schema>.search_documents.document` for most languages
//   - PGroonga over `<schema>.search_documents.raw_document` for ja/zh/ko (native script)
func Typeahead(ctx context.Context, pool *pgxpool.Pool, query string, opts search.LexicalOptions) ([]search.LexicalHit, error) {
	if isCJKLanguage(opts.Language) {
		hits, err := search.PGroongaSearch(ctx, pool, query, search.PGroongaOptions{
			Schema:      opts.Schema,
			Language:    opts.Language,
			EntityTypes: opts.EntityTypes,
			Limit:       opts.Limit,
			Prefix:      true,
		})
		if err != nil {
			return nil, err
		}
		out := make([]search.LexicalHit, 0, len(hits))
		for _, h := range hits {
			out = append(out, search.LexicalHit{
				EntityType: h.EntityType,
				EntityID:   h.EntityID,
				Language:   h.Language,
				Score:      h.Score,
			})
		}
		return out, nil
	}
	return search.LexicalSearch(ctx, pool, query, opts)
}
