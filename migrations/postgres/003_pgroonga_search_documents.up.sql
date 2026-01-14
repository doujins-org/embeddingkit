-- searchkit: add PGroonga-backed lexical search for CJK/Korean languages.
--
-- Why:
-- - Postgres FTS `simple` config doesn't provide Japanese/Chinese segmentation.
-- - Trigram/typeahead uses heavy-normalized ASCII transliteration, which is lossy for native-script queries.
--
-- This migration:
-- - Enables the `pgroonga` extension (host must allow CREATE EXTENSION).
-- - Adds PGroonga indexes for `<schema>.search_documents.raw_document` to support native-script matching.
-- - Recomputes `tsv` using `raw_document` when available (fallback to `document`) to align FTS with host-provided raw text.

BEGIN;

-- Requires superuser or appropriate privileges; hosts that cannot enable
-- extensions must handle this out-of-band and can mark this migration applied.
CREATE EXTENSION IF NOT EXISTS pgroonga;

-- Ensure FTS vectors prefer raw_document when present.
UPDATE search_documents
SET tsv = to_tsvector(
    searchkit_regconfig_for_language(language),
    coalesce(raw_document, document, '')
)
WHERE tsv IS NULL
   OR (raw_document IS NOT NULL AND btrim(raw_document) <> '' AND tsv <> to_tsvector(searchkit_regconfig_for_language(language), raw_document));

-- PGroonga full-text index for native-script queries (primary).
-- Partial index keeps size manageable while targeting languages that most need segmentation.
CREATE INDEX IF NOT EXISTS idx_search_documents_raw_document_pgroonga_cjk
    ON search_documents
 USING pgroonga (raw_document)
 WHERE language IN ('ja', 'zh', 'ko');

COMMIT;
