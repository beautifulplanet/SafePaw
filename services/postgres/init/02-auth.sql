-- =============================================================
-- SafePaw Auth Schema - Token & User Management
-- =============================================================
-- This creates the database tables needed for authentication.
-- Runs after 01-init.sql on first container start.
--
-- NOTE: The gateway uses stateless HMAC tokens for fast validation
-- (no DB hit per request). These tables are for:
--   1. Token issuance tracking (audit trail)
--   2. Token revocation (emergency kill switch)
--   3. User/service identity management
-- =============================================================

-- --------------------------------------------------------
-- Auth Users - identities that can connect to SafePaw
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS gateway.auth_users (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username    VARCHAR(128) UNIQUE NOT NULL,
    scope       VARCHAR(32) NOT NULL DEFAULT 'ws',
    enabled     BOOLEAN DEFAULT TRUE,
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- --------------------------------------------------------
-- Issued Tokens — audit trail for token creation
-- --------------------------------------------------------
-- We don't store the actual token (OPSEC: if DB is compromised,
-- attacker can't extract valid tokens). We store the hash.
CREATE TABLE IF NOT EXISTS gateway.auth_tokens (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES gateway.auth_users(id) ON DELETE CASCADE,
    token_hash      VARCHAR(64) NOT NULL,           -- SHA256 hash of the token
    scope           VARCHAR(32) NOT NULL DEFAULT 'ws',
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked         BOOLEAN DEFAULT FALSE,
    revoked_at      TIMESTAMPTZ,
    revoked_reason  VARCHAR(256),
    created_by      VARCHAR(128) DEFAULT 'system',  -- Who/what issued the token
    last_used_at    TIMESTAMPTZ,                    -- Track last usage (optional)
    CONSTRAINT token_hash_unique UNIQUE (token_hash)
);

-- --------------------------------------------------------
-- Revocation List — fast lookup for revoked tokens
-- --------------------------------------------------------
-- The gateway can periodically sync this list to memory
-- for O(1) revocation checks without DB queries per request.
CREATE TABLE IF NOT EXISTS gateway.token_revocations (
    token_hash  VARCHAR(64) PRIMARY KEY,
    revoked_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason      VARCHAR(256),
    expires_at  TIMESTAMPTZ NOT NULL  -- Auto-cleanup: remove after token would have expired anyway
);

-- --------------------------------------------------------
-- Indexes
-- --------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_auth_users_username ON gateway.auth_users(username);
CREATE INDEX IF NOT EXISTS idx_auth_users_enabled ON gateway.auth_users(enabled) WHERE enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_auth_tokens_user ON gateway.auth_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_hash ON gateway.auth_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_active ON gateway.auth_tokens(revoked, expires_at)
    WHERE revoked = FALSE;
CREATE INDEX IF NOT EXISTS idx_revocations_expires ON gateway.token_revocations(expires_at);

-- --------------------------------------------------------
-- Auto-cleanup: remove expired revocation entries
-- (Run as a periodic job or cron — keeps the table lean)
-- --------------------------------------------------------
CREATE OR REPLACE FUNCTION gateway.cleanup_expired_revocations()
RETURNS INTEGER AS $$
DECLARE
    removed INTEGER;
BEGIN
    DELETE FROM gateway.token_revocations
    WHERE expires_at < NOW();
    GET DIAGNOSTICS removed = ROW_COUNT;
    RETURN removed;
END;
$$ LANGUAGE plpgsql;

-- --------------------------------------------------------
-- Bootstrap: create a default admin user for dev
-- --------------------------------------------------------
INSERT INTO gateway.auth_users (username, scope, metadata)
VALUES ('dev-admin', 'admin', '{"note": "Default dev admin — replace in production"}')
ON CONFLICT (username) DO NOTHING;

INSERT INTO gateway.auth_users (username, scope, metadata)
VALUES ('dev-user', 'ws', '{"note": "Default dev user — replace in production"}')
ON CONFLICT (username) DO NOTHING;

-- Log successful init
DO $$
BEGIN
    RAISE NOTICE 'SafePaw auth schema initialized';
END $$;
