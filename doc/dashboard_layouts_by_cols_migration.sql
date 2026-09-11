-- Convert persisted dashboard docs from legacy breakpoint layouts to layouts_by_cols.
-- Production rollout order:
-- 1. Deploy the new frontend/backend version first so all active readers understand layouts_by_cols.
-- 2. Confirm the new version is serving traffic.
-- 3. Run this migration.
-- 4. Keep a database snapshot available for rollback.
-- This script is intentionally one-way and removes the legacy layouts key.

BEGIN;

CREATE OR REPLACE FUNCTION dashboard_layout_max_right(items jsonb)
RETURNS integer
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT COALESCE(MAX(
    COALESCE((item->>'x')::int, 0) + COALESCE((item->>'w')::int, 1)
  ), 0)
  FROM jsonb_array_elements(COALESCE(items, '[]'::jsonb)) AS item;
$$;

WITH legacy AS (
  SELECT
    id,
    doc,
    doc->'layouts' AS layouts
  FROM dashboards
  WHERE doc ? 'layouts'
    AND NOT (doc ? 'layouts_by_cols')
), converted AS (
  SELECT
    id,
    (doc - 'layouts') || jsonb_build_object(
      'layouts_by_cols',
      jsonb_build_object(
        '4', COALESCE(layouts->'xl', layouts->'lg', layouts->'md', '[]'::jsonb),
        '3', CASE
          WHEN dashboard_layout_max_right(layouts->'md') BETWEEN 1 AND 3 THEN COALESCE(layouts->'md', '[]'::jsonb)
          WHEN dashboard_layout_max_right(layouts->'lg') BETWEEN 1 AND 3 THEN COALESCE(layouts->'lg', '[]'::jsonb)
          ELSE '[]'::jsonb
        END,
        '2', COALESCE(layouts->'sm', '[]'::jsonb),
        '1', COALESCE(layouts->'xxs', layouts->'xs', '[]'::jsonb)
      )
    ) AS next_doc
  FROM legacy
)
UPDATE dashboards AS dashboards
SET
  doc = converted.next_doc,
  layout_version = dashboards.layout_version + 1,
  updated_at = now()
FROM converted
WHERE dashboards.id = converted.id;

DROP FUNCTION dashboard_layout_max_right(jsonb);

COMMIT;