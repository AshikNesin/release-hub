-- One-click keyless publishing (issue #8).
--
-- 1. Repository grants: a GitHub grant can bind a repo by its immutable ids
--    (the OIDC token's repository_id / repository_owner_id claims) plus the
--    ref it may publish from, instead of a pasted `sub` string. That removes
--    the legacy vs immutable subject format question, survives renames and
--    transfers, and can't be taken over by someone re-creating a deleted
--    owner/name. Subject grants (repository_id NULL) keep working as before;
--    a repository grant's subject column holds a synthetic, never-matched
--    `github-repository:<id>` so UNIQUE(issuer, subject) still holds.
ALTER TABLE oidc_grants ADD COLUMN repository_id INTEGER;
ALTER TABLE oidc_grants ADD COLUMN repository_owner_id INTEGER;
ALTER TABLE oidc_grants ADD COLUMN ref TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS oidc_grants_repository
    ON oidc_grants(issuer, repository_id) WHERE repository_id IS NOT NULL;

-- 2. This release hub's own GitHub App, registered through GitHub's manifest
--    flow (srv/github_app.go). One per release hub; installations are never
--    stored, the repo list is always read live from GitHub.
CREATE TABLE IF NOT EXISTS github_app (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    app_id INTEGER NOT NULL,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    owner TEXT NOT NULL DEFAULT '',       -- account or organisation login
    html_url TEXT NOT NULL DEFAULT '',
    private_key TEXT NOT NULL,            -- sealed (aes:) or a 1Password reference (op:)
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- In-flight manifest / installation round trips to github.com. state and
-- binding are stored as sha256 hex: the state travels through GitHub, the
-- binding only in an httpOnly cookie of the browser that started the flow.
CREATE TABLE IF NOT EXISTS github_flows (
    state TEXT PRIMARY KEY,
    binding TEXT NOT NULL,
    kind TEXT NOT NULL,                   -- manifest | install
    expires_at TIMESTAMP NOT NULL
);

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (012, '012-github-app');
