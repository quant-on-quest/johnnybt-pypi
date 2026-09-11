CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL DEFAULT '',
    is_admin      INTEGER NOT NULL DEFAULT 0,
    note          TEXT NOT NULL DEFAULT '',
    provider      TEXT NOT NULL DEFAULT '',
    provider_id   TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL
);

CREATE TABLE tokens (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL DEFAULT '',
    prefix       TEXT NOT NULL,
    hash         TEXT NOT NULL UNIQUE,
    scope        TEXT NOT NULL CHECK (scope IN ('read', 'write')),
    created_at   TEXT NOT NULL,
    last_used_at TEXT,
    revoked_at   TEXT
);
CREATE INDEX tokens_user_id ON tokens(user_id);

CREATE TABLE packages (
    id                       INTEGER PRIMARY KEY,
    name                     TEXT NOT NULL,
    normalized_name          TEXT NOT NULL UNIQUE,
    summary                  TEXT NOT NULL DEFAULT '',
    description              TEXT NOT NULL DEFAULT '',
    description_content_type TEXT NOT NULL DEFAULT '',
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL
);

CREATE TABLE releases (
    id            INTEGER PRIMARY KEY,
    package_id    INTEGER NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    version       TEXT NOT NULL,
    yanked        INTEGER NOT NULL DEFAULT 0,
    yanked_reason TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    UNIQUE (package_id, version)
);

CREATE TABLE files (
    id              INTEGER PRIMARY KEY,
    release_id      INTEGER NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
    filename        TEXT NOT NULL UNIQUE,
    sha256          TEXT NOT NULL,
    size            INTEGER NOT NULL,
    blob_key        TEXT NOT NULL,
    requires_python TEXT NOT NULL DEFAULT '',
    metadata_sha256 TEXT NOT NULL DEFAULT '',
    uploaded_by     INTEGER REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at     TEXT NOT NULL
);
CREATE INDEX files_release_id ON files(release_id);

CREATE TABLE entitlements (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    package_id INTEGER NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    expires_at TEXT,
    granted_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    granted_at TEXT NOT NULL,
    PRIMARY KEY (user_id, package_id)
);

CREATE TABLE downloads (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
    token_id   INTEGER REFERENCES tokens(id) ON DELETE SET NULL,
    file_id    INTEGER REFERENCES files(id) ON DELETE SET NULL,
    package_id INTEGER REFERENCES packages(id) ON DELETE SET NULL,
    filename   TEXT NOT NULL,
    ip         TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    at         TEXT NOT NULL
);
CREATE INDEX downloads_at ON downloads(at);
