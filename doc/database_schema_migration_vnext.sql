BEGIN;

ALTER TABLE workflows
	ADD COLUMN IF NOT EXISTS source_kind varchar(16),
	ADD COLUMN IF NOT EXISTS source_format varchar(32),
	ADD COLUMN IF NOT EXISTS source_code text,
	ADD COLUMN IF NOT EXISTS source_revision bigint;

UPDATE workflows
SET source_kind = 'graph'
WHERE source_kind IS NULL OR btrim(source_kind) = '';

UPDATE workflows
SET source_format = 'graph-json'
WHERE source_format IS NULL OR btrim(source_format) = '';

UPDATE workflows
SET source_code = ''
WHERE source_code IS NULL;

UPDATE workflows
SET source_revision = 1
WHERE source_revision IS NULL OR source_revision < 1;

ALTER TABLE workflows
	ALTER COLUMN source_kind SET DEFAULT 'graph',
	ALTER COLUMN source_kind SET NOT NULL,
	ALTER COLUMN source_format SET DEFAULT 'graph-json',
	ALTER COLUMN source_format SET NOT NULL,
	ALTER COLUMN source_code SET DEFAULT '',
	ALTER COLUMN source_code SET NOT NULL,
	ALTER COLUMN source_revision SET DEFAULT 1,
	ALTER COLUMN source_revision SET NOT NULL;

ALTER TABLE ers_device_bindings
	ADD COLUMN IF NOT EXISTS hdp_device_id uuid;

ALTER TABLE pending_correlations
	ADD COLUMN IF NOT EXISTS hdp_device_id uuid;

ALTER TABLE IF EXISTS device_state_points
	ADD COLUMN IF NOT EXISTS hdp_device_id uuid;

ALTER TABLE IF EXISTS hdp_device_state_points
	ADD COLUMN IF NOT EXISTS hdp_device_id uuid;

DO $$
BEGIN
	IF to_regclass('public.devices') IS NOT NULL AND to_regclass('public.hdp_devices') IS NULL THEN
		ALTER TABLE devices RENAME TO hdp_devices;
	END IF;
	IF to_regclass('public.device_states') IS NOT NULL AND to_regclass('public.hdp_device_states') IS NULL THEN
		ALTER TABLE device_states RENAME TO hdp_device_states;
	END IF;
	IF to_regclass('public.device_state_points') IS NOT NULL AND to_regclass('public.hdp_device_state_points') IS NULL THEN
		ALTER TABLE device_state_points RENAME TO hdp_device_state_points;
	END IF;
END
$$;

CREATE TABLE IF NOT EXISTS hdp_devices (
	id uuid PRIMARY KEY,
	protocol text NOT NULL,
	external_id text NOT NULL,
	name text,
	type text,
	manufacturer text,
	model text,
	description text,
	firmware text,
	icon text,
	capabilities jsonb NOT NULL DEFAULT '[]'::jsonb,
	inputs jsonb NOT NULL DEFAULT '[]'::jsonb,
	online boolean NOT NULL DEFAULT false,
	last_seen timestamptz,
	created_at timestamptz,
	updated_at timestamptz
);

CREATE TABLE IF NOT EXISTS hdp_device_states (
	device_id uuid PRIMARY KEY,
	state jsonb NOT NULL DEFAULT '{}'::jsonb,
	updated_at timestamptz
);

CREATE TABLE IF NOT EXISTS hdp_device_state_points (
	id uuid PRIMARY KEY,
	device_id text,
	hdp_device_id uuid,
	ts timestamptz,
	payload jsonb,
	topic text,
	retained boolean,
	ingested_at timestamptz
);

DO $$
BEGIN
	IF to_regclass('public.devices') IS NOT NULL THEN
		INSERT INTO hdp_devices (
			id,
			protocol,
			external_id,
			name,
			type,
			manufacturer,
			model,
			description,
			firmware,
			icon,
			capabilities,
			inputs,
			online,
			last_seen,
			created_at,
			updated_at
		)
		SELECT
			id,
			protocol,
			external_id,
			name,
			type,
			manufacturer,
			model,
			description,
			firmware,
			icon,
			COALESCE(capabilities, '[]'::jsonb),
			COALESCE(inputs, '[]'::jsonb),
			COALESCE(online, false),
			last_seen,
			created_at,
			updated_at
		FROM devices
		ON CONFLICT (id) DO UPDATE
		SET protocol = EXCLUDED.protocol,
			external_id = EXCLUDED.external_id,
			name = EXCLUDED.name,
			type = EXCLUDED.type,
			manufacturer = EXCLUDED.manufacturer,
			model = EXCLUDED.model,
			description = EXCLUDED.description,
			firmware = EXCLUDED.firmware,
			icon = EXCLUDED.icon,
			capabilities = EXCLUDED.capabilities,
			inputs = EXCLUDED.inputs,
			online = EXCLUDED.online,
			last_seen = EXCLUDED.last_seen,
			created_at = EXCLUDED.created_at,
			updated_at = EXCLUDED.updated_at;
		DROP TABLE devices;
	END IF;

	IF to_regclass('public.device_states') IS NOT NULL THEN
		INSERT INTO hdp_device_states (device_id, state, updated_at)
		SELECT device_id, COALESCE(state, '{}'::jsonb), updated_at
		FROM device_states
		ON CONFLICT (device_id) DO UPDATE
		SET state = EXCLUDED.state,
			updated_at = EXCLUDED.updated_at;
		DROP TABLE device_states;
	END IF;

	IF to_regclass('public.device_state_points') IS NOT NULL THEN
		INSERT INTO hdp_device_state_points (
			id,
			device_id,
			hdp_device_id,
			ts,
			payload,
			topic,
			retained,
			ingested_at
		)
		SELECT
			id,
			device_id,
			hdp_device_id,
			ts,
			payload,
			topic,
			retained,
			ingested_at
		FROM device_state_points
		ON CONFLICT (id) DO UPDATE
		SET device_id = EXCLUDED.device_id,
			hdp_device_id = EXCLUDED.hdp_device_id,
			ts = EXCLUDED.ts,
			payload = EXCLUDED.payload,
			topic = EXCLUDED.topic,
			retained = EXCLUDED.retained,
			ingested_at = EXCLUDED.ingested_at;
		DROP TABLE device_state_points;
	END IF;
END
$$;

UPDATE hdp_devices
SET capabilities = '[]'::jsonb
WHERE capabilities IS NULL;

UPDATE hdp_devices
SET inputs = '[]'::jsonb
WHERE inputs IS NULL;

UPDATE hdp_devices
SET online = false
WHERE online IS NULL;

ALTER TABLE hdp_devices
	ALTER COLUMN capabilities SET DEFAULT '[]'::jsonb,
	ALTER COLUMN capabilities SET NOT NULL,
	ALTER COLUMN inputs SET DEFAULT '[]'::jsonb,
	ALTER COLUMN inputs SET NOT NULL,
	ALTER COLUMN online SET DEFAULT false,
	ALTER COLUMN online SET NOT NULL;

UPDATE hdp_device_states
SET state = '{}'::jsonb
WHERE state IS NULL;

ALTER TABLE hdp_device_states
	ALTER COLUMN state SET DEFAULT '{}'::jsonb,
	ALTER COLUMN state SET NOT NULL;

DO $$
BEGIN
	IF to_regclass('public.idx_hdp_devices_protocol_external') IS NOT NULL AND to_regclass('public.idx_devices_protocol_external') IS NULL THEN
		ALTER INDEX idx_hdp_devices_protocol_external RENAME TO idx_devices_protocol_external;
	END IF;
	IF to_regclass('public.idx_hdp_device_state_points_device_ts') IS NOT NULL AND to_regclass('public.idx_device_ts') IS NULL THEN
		ALTER INDEX idx_hdp_device_state_points_device_ts RENAME TO idx_device_ts;
	END IF;
	IF to_regclass('public.idx_device_state_points_hdp_device_id') IS NOT NULL AND to_regclass('public.idx_hdp_device_state_points_hdp_device_id') IS NULL THEN
		ALTER INDEX idx_device_state_points_hdp_device_id RENAME TO idx_hdp_device_state_points_hdp_device_id;
	END IF;
END
$$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_protocol_external ON hdp_devices (protocol, external_id);
CREATE INDEX IF NOT EXISTS idx_device_ts ON hdp_device_state_points (device_id, ts);
CREATE INDEX IF NOT EXISTS idx_hdp_device_state_points_hdp_device_id ON hdp_device_state_points (hdp_device_id);

UPDATE ers_device_bindings AS binding
SET hdp_device_id = device.id
FROM hdp_devices AS device
WHERE binding.kind = 'hdp'
	AND binding.hdp_device_id IS NULL
	AND position('/' in binding.external_id) > 0
	AND split_part(binding.external_id, '/', 1) = device.protocol
	AND substring(binding.external_id from position('/' in binding.external_id) + 1) = device.external_id;

UPDATE pending_correlations AS pending
SET hdp_device_id = device.id
FROM hdp_devices AS device
WHERE pending.hdp_device_id IS NULL
	AND position('/' in pending.device_id) > 0
	AND split_part(pending.device_id, '/', 1) = device.protocol
	AND substring(pending.device_id from position('/' in pending.device_id) + 1) = device.external_id;

UPDATE hdp_device_state_points AS point
SET hdp_device_id = device.id
FROM hdp_devices AS device
WHERE point.hdp_device_id IS NULL
	AND position('/' in point.device_id) > 0
	AND split_part(point.device_id, '/', 1) = device.protocol
	AND substring(point.device_id from position('/' in point.device_id) + 1) = device.external_id;

COMMIT;
