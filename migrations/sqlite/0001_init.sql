-- +goose Up
CREATE TABLE users (
    id              TEXT PRIMARY KEY,
    oidc_issuer     TEXT NOT NULL,
    oidc_subject    TEXT NOT NULL,
    email           TEXT NOT NULL DEFAULT '',
    display_name    TEXT,
    role            TEXT NOT NULL DEFAULT 'viewer' CHECK (role IN ('viewer', 'member', 'admin')),
    last_login_at   TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (oidc_issuer, oidc_subject)
) STRICT;

CREATE TABLE sessions (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    expires_at      TEXT NOT NULL,
    last_seen_at    TEXT,
    ip              TEXT,
    user_agent      TEXT,
    revoked_at      TEXT
) STRICT;

CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE domains (
    id                      TEXT PRIMARY KEY,
    name_ace                TEXT NOT NULL UNIQUE,
    name_unicode            TEXT NOT NULL,
    registrar               TEXT,
    expires_at              TEXT,
    state                   TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'paused', 'archived')),
    inherit_global_alerts   INTEGER NOT NULL DEFAULT 1 CHECK (inherit_global_alerts IN (0, 1)),
    muted_until             TEXT,
    mute_reason             TEXT,
    check_interval_seconds  INTEGER NOT NULL DEFAULT 86400 CHECK (check_interval_seconds >= 300),
    next_check_at           TEXT,
    last_checked_at         TEXT,
    last_check_status       TEXT,
    last_check_error        TEXT,
    expiry_confidence       TEXT NOT NULL DEFAULT 'unknown' CHECK (expiry_confidence IN ('high', 'medium', 'low', 'unknown')),
    consecutive_failures    INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    version                 INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    notes                   TEXT,
    created_by              TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    archived_at             TEXT
) STRICT;

CREATE INDEX idx_domains_due ON domains(next_check_at) WHERE state = 'active' AND next_check_at IS NOT NULL;
CREATE INDEX idx_domains_expiry ON domains(expires_at) WHERE state = 'active';
CREATE INDEX idx_domains_state ON domains(state, archived_at);

CREATE TABLE domain_checks (
    id              TEXT PRIMARY KEY,
    domain_id       TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    source          TEXT NOT NULL CHECK (source IN ('rdap', 'whois')),
    status          TEXT NOT NULL CHECK (status IN ('ok', 'error', 'unsupported')),
    http_status     INTEGER,
    expires_at      TEXT,
    registrar       TEXT,
    confidence      TEXT NOT NULL CHECK (confidence IN ('high', 'medium', 'low', 'unknown')),
    error_code      TEXT,
    error_message   TEXT,
    duration_ms     INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    started_at      TEXT NOT NULL,
    finished_at     TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE INDEX idx_domain_checks_domain ON domain_checks(domain_id, created_at DESC);
CREATE INDEX idx_domain_checks_started ON domain_checks(started_at);

CREATE TABLE check_raw_responses (
    check_id        TEXT PRIMARY KEY REFERENCES domain_checks(id) ON DELETE CASCADE,
    encoding        TEXT NOT NULL,
    body            BLOB NOT NULL,
    content_type    TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    expires_at      TEXT
) STRICT;

CREATE INDEX idx_check_raw_expires ON check_raw_responses(expires_at) WHERE expires_at IS NOT NULL;

CREATE TABLE channels (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL UNIQUE,
    type                    TEXT NOT NULL CHECK (type IN ('email', 'webhook', 'discord', 'telegram')),
    config_ciphertext       TEXT NOT NULL,
    enabled                 INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    receive_system_alerts   INTEGER NOT NULL DEFAULT 0 CHECK (receive_system_alerts IN (0, 1)),
    muted_until             TEXT,
    mute_reason             TEXT,
    archived_at             TEXT,
    created_by              TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE INDEX idx_channels_type ON channels(type) WHERE archived_at IS NULL;

CREATE TABLE alert_rules (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    scope               TEXT NOT NULL CHECK (scope IN ('global', 'domain')),
    domain_id           TEXT REFERENCES domains(id) ON DELETE CASCADE,
    offset_value        INTEGER NOT NULL CHECK (offset_value >= 0),
    offset_unit         TEXT NOT NULL CHECK (offset_unit IN ('days', 'months')),
    direction           TEXT NOT NULL CHECK (direction IN ('before', 'after')),
    send_time_local     TEXT NOT NULL DEFAULT '09:00',
    enabled             INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    archived_at         TEXT,
    created_by          TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (
        (scope = 'global' AND domain_id IS NULL) OR
        (scope = 'domain' AND domain_id IS NOT NULL)
    ),
    CHECK (offset_value > 0 OR direction = 'before')
) STRICT;

CREATE INDEX idx_alert_rules_domain ON alert_rules(domain_id) WHERE archived_at IS NULL;

CREATE TABLE alert_rule_channels (
    rule_id     TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    channel_id  TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    PRIMARY KEY (rule_id, channel_id)
) STRICT;

CREATE INDEX idx_alert_rule_channels_channel ON alert_rule_channels(channel_id);

CREATE TABLE incidents (
    id              TEXT PRIMARY KEY,
    domain_id       TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('check_failure', 'parse_failure', 'expiry_unknown')),
    state           TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'resolved')),
    opened_at       TEXT NOT NULL,
    resolved_at     TEXT,
    last_seen_at    TEXT NOT NULL,
    details_json    TEXT NOT NULL DEFAULT '{}',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE UNIQUE INDEX idx_incidents_open ON incidents(domain_id, kind) WHERE state = 'open';
CREATE INDEX idx_incidents_domain ON incidents(domain_id, opened_at DESC);

CREATE TABLE notifications (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL CHECK (kind IN ('expiry', 'system')),
    domain_id       TEXT REFERENCES domains(id) ON DELETE CASCADE,
    incident_id     TEXT REFERENCES incidents(id) ON DELETE SET NULL,
    rule_id         TEXT REFERENCES alert_rules(id) ON DELETE SET NULL,
    channel_id      TEXT REFERENCES channels(id) ON DELETE SET NULL,
    expiry_date_utc TEXT,
    dedupe_key      TEXT NOT NULL UNIQUE,
    state           TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'sending', 'sent', 'failed', 'suppressed', 'canceled')),
    scheduled_at    TEXT NOT NULL,
    next_attempt_at TEXT,
    attempts        INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error      TEXT,
    payload_json    TEXT NOT NULL DEFAULT '{}',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    sent_at         TEXT
) STRICT;

CREATE INDEX idx_notifications_due ON notifications(next_attempt_at) WHERE state IN ('pending', 'sending');
CREATE INDEX idx_notifications_domain ON notifications(domain_id, created_at DESC);
CREATE INDEX idx_notifications_channel ON notifications(channel_id, created_at DESC);

CREATE TABLE delivery_attempts (
    id              TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    attempt_no      INTEGER NOT NULL CHECK (attempt_no > 0),
    started_at      TEXT NOT NULL,
    finished_at     TEXT,
    status          TEXT NOT NULL CHECK (status IN ('success', 'retry', 'failed')),
    response_code   INTEGER,
    error           TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (notification_id, attempt_no)
) STRICT;

CREATE INDEX idx_delivery_attempts_notification ON delivery_attempts(notification_id, started_at DESC);

CREATE TABLE jobs (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    payload_json    TEXT NOT NULL DEFAULT '{}',
    state           TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'running', 'succeeded', 'failed', 'dead', 'canceled')),
    run_at          TEXT NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts    INTEGER NOT NULL DEFAULT 10 CHECK (max_attempts > 0),
    locked_by       TEXT,
    locked_until    TEXT,
    last_error      TEXT,
    unique_key      TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    finished_at     TEXT
) STRICT;

CREATE INDEX idx_jobs_due ON jobs(run_at) WHERE state = 'pending';
CREATE UNIQUE INDEX idx_jobs_unique ON jobs(unique_key) WHERE unique_key IS NOT NULL AND state IN ('pending', 'running');

CREATE TABLE settings (
    key                 TEXT PRIMARY KEY,
    value_json          TEXT,
    value_ciphertext    TEXT,
    is_secret           INTEGER NOT NULL DEFAULT 0 CHECK (is_secret IN (0, 1)),
    updated_by          TEXT REFERENCES users(id) ON DELETE SET NULL,
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (is_secret = 0 OR value_ciphertext IS NOT NULL)
) STRICT;

CREATE TABLE audit_log (
    id              TEXT PRIMARY KEY,
    actor_user_id   TEXT REFERENCES users(id) ON DELETE SET NULL,
    action          TEXT NOT NULL,
    entity_type     TEXT NOT NULL,
    entity_id       TEXT,
    before_json     TEXT,
    after_json      TEXT,
    ip              TEXT,
    user_agent      TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE INDEX idx_audit_created ON audit_log(created_at DESC);
CREATE INDEX idx_audit_entity ON audit_log(entity_type, entity_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS delivery_attempts;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS incidents;
DROP TABLE IF EXISTS alert_rule_channels;
DROP TABLE IF EXISTS alert_rules;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS check_raw_responses;
DROP TABLE IF EXISTS domain_checks;
DROP TABLE IF EXISTS domains;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
