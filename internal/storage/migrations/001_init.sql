CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS auth_users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS auth_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES auth_users(id),
    role TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expiry ON auth_sessions(expires_at);

INSERT OR IGNORE INTO auth_users(id, username, password_hash, role, created_at) VALUES
 ('u-auditor','auditor','8c2b88aed9b74485e069701da4059b6c2399f6a92932de97ba9ef7acf895b4fd','auditor','2026-01-01T00:00:00Z'),
 ('u-labreviewer','labreviewer','74f58f398e32ae7816819df68fe8fba9e16dc664e5bec42812953bad90d4075a','labreviewer','2026-01-01T00:00:00Z'),
 ('u-vendorengineer','vendorengineer','fe8d23935a744a0f4775f4b12cbc766a3761726739d365f2c5554eb9f63116fa','vendorengineer','2026-01-01T00:00:00Z'),
 ('u-testengineer','testengineer','ea43d511bede0a641bdd0a5c0dc5358de0b03b38a43842f86b431a8f8688e677','testengineer','2026-01-01T00:00:00Z');

CREATE TABLE IF NOT EXISTS charging_products (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, market TEXT NOT NULL DEFAULT '', vendor_id TEXT NOT NULL,
 status TEXT NOT NULL, supported_protocols TEXT NOT NULL, max_power_watts INTEGER NOT NULL,
 gan INTEGER NOT NULL DEFAULT 0, port_count INTEGER NOT NULL DEFAULT 1,
 thermal_limit_c REAL NOT NULL DEFAULT 55, battery_architecture TEXT NOT NULL DEFAULT '',
 cell_count INTEGER NOT NULL DEFAULT 1,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS charging_issues (
 id TEXT PRIMARY KEY, product_id TEXT NOT NULL REFERENCES charging_products(id), kind TEXT NOT NULL,
 severity TEXT NOT NULL DEFAULT 'medium', description TEXT NOT NULL, state TEXT NOT NULL,
 submitted_by TEXT NOT NULL, vendor_assignee_id TEXT NOT NULL DEFAULT '', review_due_at TEXT NOT NULL,
 mitigated_at TEXT, certified_at TEXT, firmware_evidence TEXT NOT NULL DEFAULT '', version INTEGER NOT NULL DEFAULT 1,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_charging_issues_product_state ON charging_issues(product_id,state);
CREATE INDEX IF NOT EXISTS idx_charging_issues_due ON charging_issues(review_due_at,state);
CREATE TABLE IF NOT EXISTS charging_charge_tests (
 id TEXT PRIMARY KEY, product_id TEXT NOT NULL REFERENCES charging_products(id), lab_engineer_id TEXT NOT NULL,
 checked_at TEXT NOT NULL, protocol_handshake_ok INTEGER NOT NULL, cable_certificate_expiry TEXT NOT NULL,
 thermal_control_ok INTEGER NOT NULL, power_display_ok INTEGER NOT NULL, notes TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS shard_meta (
    shard_id     TEXT PRIMARY KEY,
    date         TEXT NOT NULL,
    protocol_id     TEXT NOT NULL,
    file_path    TEXT NOT NULL,
    record_count INTEGER NOT NULL DEFAULT 0,
    checksum     TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'ok',
    data_version INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shard_date ON shard_meta(date);
CREATE INDEX IF NOT EXISTS idx_shard_protocol ON shard_meta(protocol_id);
CREATE INDEX IF NOT EXISTS idx_shard_status ON shard_meta(status);

CREATE TABLE IF NOT EXISTS session_items (
    id             TEXT PRIMARY KEY,
    session_no        TEXT UNIQUE NOT NULL,
    protocol_id       TEXT NOT NULL,
    adapter_model     TEXT,
    state          TEXT NOT NULL,
    handshake_id    TEXT,
    mitigation_id TEXT,
    device_model TEXT,
    charger_model   TEXT,
    vendor_id    TEXT,
    cable_id    TEXT,
    lab_id  TEXT,
    firmware_version  TEXT,
    owner_id    TEXT,
    requested_at  TEXT NOT NULL,
    capability_checked_at      TEXT,
    negotiating_at  TEXT,
    charging_at     TEXT,
    signed_at      TEXT,
    completed_at   TEXT,
    version        INTEGER NOT NULL DEFAULT 1,
    shard_id       TEXT NOT NULL,
    data_version   INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_session_state ON session_items(state);
CREATE INDEX IF NOT EXISTS idx_session_protocol ON session_items(protocol_id);
CREATE INDEX IF NOT EXISTS idx_session_vehicle ON session_items(adapter_model);
CREATE INDEX IF NOT EXISTS idx_session_handshake ON session_items(handshake_id);

CREATE TABLE IF NOT EXISTS handshake_forms (
    id                TEXT PRIMARY KEY,
    form_no           TEXT UNIQUE NOT NULL,
    date              TEXT NOT NULL,
    protocol_id          TEXT NOT NULL,
    adapter_model        TEXT,
    state             TEXT NOT NULL,
    outbound_product  TEXT,
    outbound_signer   TEXT,
    outbound_signed_at TEXT,
    arrival_product   TEXT,
    arrival_signer    TEXT,
    arrival_signed_at TEXT,
    session_item_count   INTEGER NOT NULL DEFAULT 0,
    owner_id       TEXT,
    requested_at     TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    version           INTEGER NOT NULL DEFAULT 1,
    shard_id          TEXT NOT NULL,
    data_version      INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_handshake_state ON handshake_forms(state);
CREATE INDEX IF NOT EXISTS idx_handshake_date ON handshake_forms(date);
CREATE INDEX IF NOT EXISTS idx_handshake_protocol ON handshake_forms(protocol_id);

CREATE TABLE IF NOT EXISTS mitigation_requests (
    id               TEXT PRIMARY KEY,
    request_no       TEXT UNIQUE NOT NULL,
    session_id          TEXT NOT NULL,
    session_no          TEXT NOT NULL,
    type             TEXT NOT NULL,
    target_address   TEXT,
    state            TEXT NOT NULL,
    submitted_by     TEXT NOT NULL,
    submitted_at     TEXT NOT NULL,
    reviewed_by      TEXT,
    reviewed_at      TEXT,
    review_note      TEXT,
    issued_by        TEXT,
    issued_at        TEXT,
    executed_at      TEXT,
    completed_at     TEXT,
    withdrawn_by     TEXT,
    withdrawn_at     TEXT,
    withdrawn_reason TEXT,
    conflict_reason  TEXT,
    lost_at          TEXT,
    version          INTEGER NOT NULL DEFAULT 1,
    shard_id         TEXT NOT NULL,
    data_version     INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_disp_state ON mitigation_requests(state);
CREATE INDEX IF NOT EXISTS idx_disp_session ON mitigation_requests(session_id);

CREATE TABLE IF NOT EXISTS active_mitigations (
    session_id        TEXT PRIMARY KEY,
    mitigation_id TEXT NOT NULL,
    created_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS batch_records (
    id              TEXT PRIMARY KEY,
    adapter_model      TEXT NOT NULL,
    date            TEXT NOT NULL,
    protocol_id        TEXT NOT NULL,
    state           TEXT NOT NULL,
    total_count     INTEGER NOT NULL DEFAULT 0,
    succeeded_count INTEGER NOT NULL DEFAULT 0,
    failed_count    INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    version         INTEGER NOT NULL DEFAULT 1,
    shard_id        TEXT NOT NULL,
    data_version    INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_batch_state ON batch_records(state);
CREATE INDEX IF NOT EXISTS idx_batch_vehicle ON batch_records(adapter_model);

CREATE TABLE IF NOT EXISTS batch_items (
    id         TEXT PRIMARY KEY,
    batch_id   TEXT NOT NULL,
    session_id    TEXT NOT NULL,
    session_no    TEXT NOT NULL,
    state      TEXT NOT NULL,
    error      TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_batch_item_batch ON batch_items(batch_id);
CREATE INDEX IF NOT EXISTS idx_batch_item_state ON batch_items(state);

CREATE TABLE IF NOT EXISTS telemetry_entries (
    id          TEXT PRIMARY KEY,
    date        TEXT NOT NULL,
    protocol_id    TEXT NOT NULL,
    volume_no   TEXT NOT NULL,
    form_no     TEXT,
    entry_type  TEXT NOT NULL,
    session_no     TEXT,
    owner_id TEXT,
    description TEXT,
    prev_state  TEXT,
    next_state  TEXT,
    created_at  TEXT NOT NULL,
    shard_id    TEXT NOT NULL,
    data_version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_telemetry_date ON telemetry_entries(date);
CREATE INDEX IF NOT EXISTS idx_telemetry_protocol ON telemetry_entries(protocol_id);
CREATE INDEX IF NOT EXISTS idx_telemetry_owner_id ON telemetry_entries(owner_id);
CREATE INDEX IF NOT EXISTS idx_telemetry_volume ON telemetry_entries(volume_no);

CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    type        TEXT NOT NULL,
    business_key TEXT NOT NULL,
    shard_id    TEXT,
    payload     TEXT NOT NULL,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);

CREATE TABLE IF NOT EXISTS subscriber_checkpoints (
    id             TEXT PRIMARY KEY,
    type           TEXT NOT NULL,
    name           TEXT,
    last_event_id  INTEGER NOT NULL DEFAULT 0,
    last_active_at TEXT NOT NULL,
    created_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_trail (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    actor        TEXT NOT NULL,
    action       TEXT NOT NULL,
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    shard_id     TEXT,
    before_state TEXT,
    after_state  TEXT,
    detail       TEXT,
    timestamp    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_trail(actor);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_trail(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_trail(timestamp);

CREATE TABLE IF NOT EXISTS permanent_failures (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    task_type      TEXT NOT NULL,
    entity_id      TEXT NOT NULL,
    shard_id       TEXT,
    last_error     TEXT,
    attempts       INTEGER NOT NULL DEFAULT 0,
    max_attempts   INTEGER NOT NULL DEFAULT 3,
    last_attempt_at TEXT,
    next_retry_at  TEXT,
    status         TEXT NOT NULL DEFAULT 'pending',
    created_at     TEXT NOT NULL,
    resolved_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_failure_status ON permanent_failures(status);
CREATE INDEX IF NOT EXISTS idx_failure_task ON permanent_failures(task_type);
