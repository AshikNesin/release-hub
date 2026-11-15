-- Keyless publishing: OIDC grants bind one external identity (a GitHub repo's
-- Actions subject) to one app, so CI can exchange a short-lived OIDC token
-- for a hub token with no stored secret. One repo → one app (UNIQUE on the
-- identity), and the minted token is scoped to that app's publish endpoints.

CREATE TABLE IF NOT EXISTS oidc_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    issuer TEXT NOT NULL,                    -- token issuer (iss claim), e.g. https://token.actions.githubusercontent.com
    subject TEXT NOT NULL,                   -- subject (sub claim), e.g. repo:ashiknesin/release-hub:ref:refs/heads/main
    name TEXT NOT NULL DEFAULT '',           -- free-form label for the UI
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    UNIQUE(issuer, subject)                  -- one identity grants access to exactly one app
);

-- Minted exchange tokens are api_tokens with a scope and an expiry; a row
-- also remembers which grant minted it, so revoking the grant kills its
-- outstanding tokens immediately. scope_slug '' = hub-wide (handmade tokens).
ALTER TABLE api_tokens ADD COLUMN scope_slug TEXT NOT NULL DEFAULT '';
ALTER TABLE api_tokens ADD COLUMN expires_at TIMESTAMP;
ALTER TABLE api_tokens ADD COLUMN oidc_grant_id INTEGER REFERENCES oidc_grants(id) ON DELETE SET NULL;

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (011, '011-oidc-grants');
