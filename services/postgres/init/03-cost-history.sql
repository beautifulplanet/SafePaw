-- =============================================================
-- SafePaw Cost History Schema — Usage Persistence
-- =============================================================
-- Stores daily cost/token snapshots from the gateway's
-- OpenClaw usage collector. The wizard writes snapshots here
-- every poll cycle; the dashboard reads historical data back.
--
-- Phase 2 of cost monitoring: provides trend analysis,
-- per-model breakdowns, and anomaly detection data.
-- =============================================================

-- --------------------------------------------------------
-- Daily cost snapshots — one row per day
-- --------------------------------------------------------
-- Persists the rolling 30-day window from usage.cost so
-- data survives gateway restarts and enables historical
-- queries beyond the live window.
CREATE TABLE IF NOT EXISTS gateway.cost_daily_snapshots (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    date            DATE NOT NULL,
    input_tokens    BIGINT NOT NULL DEFAULT 0,
    output_tokens   BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens  BIGINT NOT NULL DEFAULT 0,
    cache_write_tokens BIGINT NOT NULL DEFAULT 0,
    total_tokens    BIGINT NOT NULL DEFAULT 0,
    total_cost_usd  NUMERIC(12,6) NOT NULL DEFAULT 0,
    input_cost_usd  NUMERIC(12,6) NOT NULL DEFAULT 0,
    output_cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0,
    cache_read_cost_usd  NUMERIC(12,6) NOT NULL DEFAULT 0,
    cache_write_cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0,
    messages_total  INTEGER NOT NULL DEFAULT 0,
    messages_user   INTEGER NOT NULL DEFAULT 0,
    messages_assistant INTEGER NOT NULL DEFAULT 0,
    tool_calls      INTEGER NOT NULL DEFAULT 0,
    errors          INTEGER NOT NULL DEFAULT 0,
    snapshot_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_daily_date_unique UNIQUE (date)
);

-- --------------------------------------------------------
-- Per-model cost snapshots — one row per model per day
-- --------------------------------------------------------
-- From sessions.usage aggregates.byModel: shows which
-- models are consuming cost. Essential for heartbeat waste
-- detection (high-frequency, low-token calls to expensive models).
CREATE TABLE IF NOT EXISTS gateway.cost_model_snapshots (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    date            DATE NOT NULL,
    provider        VARCHAR(64) NOT NULL DEFAULT '',
    model           VARCHAR(128) NOT NULL DEFAULT '',
    request_count   INTEGER NOT NULL DEFAULT 0,
    input_tokens    BIGINT NOT NULL DEFAULT 0,
    output_tokens   BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens  BIGINT NOT NULL DEFAULT 0,
    cache_write_tokens BIGINT NOT NULL DEFAULT 0,
    total_tokens    BIGINT NOT NULL DEFAULT 0,
    total_cost_usd  NUMERIC(12,6) NOT NULL DEFAULT 0,
    snapshot_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_model_date_unique UNIQUE (date, provider, model)
);

-- --------------------------------------------------------
-- Indexes
-- --------------------------------------------------------
-- Time-range queries on daily snapshots (the primary access pattern)
CREATE INDEX IF NOT EXISTS idx_cost_daily_date
    ON gateway.cost_daily_snapshots(date DESC);

-- Model breakdown queries: by date range, by model
CREATE INDEX IF NOT EXISTS idx_cost_model_date
    ON gateway.cost_model_snapshots(date DESC);
CREATE INDEX IF NOT EXISTS idx_cost_model_provider_model
    ON gateway.cost_model_snapshots(provider, model, date DESC);

-- --------------------------------------------------------
-- Auto-cleanup: remove snapshots older than retention period
-- Default: 365 days (configurable via the retention_days param)
-- --------------------------------------------------------
CREATE OR REPLACE FUNCTION gateway.cleanup_old_cost_snapshots(retention_days INTEGER DEFAULT 365)
RETURNS INTEGER AS $$
DECLARE
    removed INTEGER := 0;
    r INTEGER;
BEGIN
    DELETE FROM gateway.cost_daily_snapshots
    WHERE date < CURRENT_DATE - retention_days;
    GET DIAGNOSTICS r = ROW_COUNT;
    removed := removed + r;

    DELETE FROM gateway.cost_model_snapshots
    WHERE date < CURRENT_DATE - retention_days;
    GET DIAGNOSTICS r = ROW_COUNT;
    removed := removed + r;

    RETURN removed;
END;
$$ LANGUAGE plpgsql;

-- Log successful init
DO $$
BEGIN
    RAISE NOTICE 'SafePaw cost history schema initialized';
END $$;
