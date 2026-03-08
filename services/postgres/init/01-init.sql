-- =============================================================
-- SafePaw PostgreSQL Initialization
-- =============================================================
-- This runs ONCE on first container start (empty data volume).
-- Postgres auto-creates the DB from POSTGRES_DB env var,
-- so we just set up schemas, extensions, and hardening here.
-- =============================================================

-- Enable UUID generation (needed for primary keys)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create schemas for service isolation (defense in depth)
CREATE SCHEMA IF NOT EXISTS gateway;
CREATE SCHEMA IF NOT EXISTS router;
CREATE SCHEMA IF NOT EXISTS agent;

-- Sessions table (Gateway tracks active WebSocket connections)
CREATE TABLE IF NOT EXISTS gateway.sessions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL,
    channel     VARCHAR(64) NOT NULL,
    connected   BOOLEAN DEFAULT TRUE,
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Channel configs (Router needs to know channel routing rules)
CREATE TABLE IF NOT EXISTS router.channel_configs (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    channel     VARCHAR(64) UNIQUE NOT NULL,
    agent_id    UUID,
    config      JSONB DEFAULT '{}',
    enabled     BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Agent state (Agent service persists conversation context)
CREATE TABLE IF NOT EXISTS agent.conversations (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id      UUID NOT NULL,
    channel         VARCHAR(64) NOT NULL,
    message_count   INTEGER DEFAULT 0,
    context         JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for frequent queries
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON gateway.sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_channel ON gateway.sessions(channel);
CREATE INDEX IF NOT EXISTS idx_channel_configs_channel ON router.channel_configs(channel);
CREATE INDEX IF NOT EXISTS idx_conversations_session ON agent.conversations(session_id);

-- Revoke public access (defense in depth — only the app user connects)
REVOKE ALL ON SCHEMA public FROM PUBLIC;

-- Log successful init
DO $$
BEGIN
    RAISE NOTICE 'SafePaw database initialized successfully';
END $$;
