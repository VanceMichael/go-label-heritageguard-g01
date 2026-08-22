CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    timezone TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('registrar','conservator','coordinator','supervisor')),
    password_hash BLOB NOT NULL,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0,1)),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (tenant_id, email)
);
CREATE INDEX idx_users_tenant_active ON users(tenant_id, active, role);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_sessions_user_active ON sessions(tenant_id, user_id, revoked_at, expires_at);

CREATE TABLE artifacts (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    accession_no TEXT NOT NULL,
    name TEXT NOT NULL,
    material TEXT NOT NULL,
    period TEXT NOT NULL,
    risk_class TEXT NOT NULL CHECK (risk_class IN ('low','moderate','high','critical')),
    status TEXT NOT NULL CHECK (status IN ('registered','assessment','quarantined','treatment','ready','on_display','on_loan','archived')),
    current_zone_id TEXT NOT NULL,
    current_case_id TEXT,
    active_loan_id TEXT,
    last_report_id TEXT,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (tenant_id, accession_no)
);
CREATE INDEX idx_artifacts_tenant_status ON artifacts(tenant_id, status, updated_at DESC);
CREATE INDEX idx_artifacts_zone ON artifacts(tenant_id, current_zone_id, status);

CREATE TABLE condition_reports (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    inspector_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    summary TEXT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('low','moderate','high','critical')),
    measurements_json TEXT NOT NULL,
    observed_issues_json TEXT NOT NULL,
    final INTEGER NOT NULL CHECK (final IN (0,1)),
    created_at TEXT NOT NULL
);
CREATE INDEX idx_condition_artifact_time ON condition_reports(tenant_id, artifact_id, created_at DESC);

CREATE TABLE quarantine_zones (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    capacity INTEGER NOT NULL CHECK (capacity > 0),
    occupied INTEGER NOT NULL DEFAULT 0 CHECK (occupied >= 0 AND occupied <= capacity),
    version INTEGER NOT NULL DEFAULT 1,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0,1)),
    UNIQUE (tenant_id, name)
);

CREATE TABLE quarantine_cases (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    zone_id TEXT NOT NULL REFERENCES quarantine_zones(id) ON DELETE RESTRICT,
    reason TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('open','treating','resolved')),
    opened_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    resolved_by TEXT REFERENCES users(id) ON DELETE RESTRICT,
    version INTEGER NOT NULL DEFAULT 1,
    opened_at TEXT NOT NULL,
    resolved_at TEXT
);
CREATE UNIQUE INDEX idx_quarantine_one_active ON quarantine_cases(tenant_id, artifact_id) WHERE status != 'resolved';

CREATE TABLE treatment_plans (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    quarantine_id TEXT NOT NULL REFERENCES quarantine_cases(id) ON DELETE RESTRICT,
    conservator_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    procedure TEXT NOT NULL,
    evidence_uri TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('draft','approved','in_progress','completed','rejected')),
    version INTEGER NOT NULL DEFAULT 1,
    approved_by TEXT REFERENCES users(id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT
);
CREATE INDEX idx_treatment_queue ON treatment_plans(tenant_id, status, updated_at);

CREATE TABLE display_cases (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    gallery TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('available','reserved','active','incident','offline')),
    artifact_id TEXT REFERENCES artifacts(id) ON DELETE RESTRICT,
    min_humidity REAL NOT NULL,
    max_humidity REAL NOT NULL,
    min_temp_c REAL NOT NULL,
    max_temp_c REAL NOT NULL,
    reservation_to TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL,
    CHECK (min_humidity < max_humidity),
    CHECK (min_temp_c < max_temp_c),
    UNIQUE (tenant_id, gallery, name)
);
CREATE INDEX idx_cases_available ON display_cases(tenant_id, status, gallery);

CREATE TABLE installations (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    display_case_id TEXT NOT NULL REFERENCES display_cases(id) ON DELETE RESTRICT,
    mount_verified INTEGER NOT NULL CHECK (mount_verified IN (0,1)),
    seal_verified INTEGER NOT NULL CHECK (seal_verified IN (0,1)),
    environment_ready INTEGER NOT NULL CHECK (environment_ready IN (0,1)),
    installed_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    installed_at TEXT NOT NULL,
    UNIQUE (tenant_id, artifact_id, display_case_id, installed_at)
);

CREATE TABLE environment_readings (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    display_case_id TEXT NOT NULL REFERENCES display_cases(id) ON DELETE RESTRICT,
    device_id TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence >= 0),
    temperature_c REAL NOT NULL,
    humidity REAL NOT NULL CHECK (humidity >= 0 AND humidity <= 100),
    observed_at TEXT NOT NULL,
    received_at TEXT NOT NULL,
    UNIQUE (tenant_id, device_id, sequence)
);
CREATE INDEX idx_readings_case_time ON environment_readings(tenant_id, display_case_id, observed_at DESC);

CREATE TABLE incidents (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    display_case_id TEXT NOT NULL REFERENCES display_cases(id) ON DELETE RESTRICT,
    artifact_id TEXT REFERENCES artifacts(id) ON DELETE RESTRICT,
    window_key TEXT NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('open','responding','monitoring','closed')),
    summary TEXT NOT NULL,
    remediated INTEGER NOT NULL DEFAULT 0 CHECK (remediated IN (0,1)),
    version INTEGER NOT NULL DEFAULT 1,
    opened_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    closed_at TEXT,
    UNIQUE (tenant_id, display_case_id, window_key, kind)
);
CREATE INDEX idx_incidents_active ON incidents(tenant_id, status, updated_at DESC);

CREATE TABLE loan_requests (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    borrower TEXT NOT NULL,
    purpose TEXT NOT NULL,
    start_at TEXT NOT NULL,
    end_at TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft','submitted','approved','packed','dispatched','returning','returned','rejected','cancelled')),
    courier_reference TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    approved_by TEXT REFERENCES users(id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (start_at < end_at)
);
CREATE INDEX idx_loans_artifact_period ON loan_requests(tenant_id, artifact_id, start_at, end_at, status);

CREATE TABLE custody_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    loan_id TEXT REFERENCES loan_requests(id) ON DELETE RESTRICT,
    from_holder TEXT NOT NULL,
    to_holder TEXT NOT NULL,
    location TEXT NOT NULL,
    seal_number TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    recorded_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT
);
CREATE INDEX idx_custody_artifact_time ON custody_events(tenant_id, artifact_id, occurred_at DESC);

CREATE TABLE idempotency_keys (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    request_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_status INTEGER,
    response_body BLOB,
    resource_id TEXT,
    state TEXT NOT NULL CHECK (state IN ('started','completed')),
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (tenant_id, method, path, request_key)
);

CREATE TABLE worker_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload BLOB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','running','succeeded','retry','failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    available_at TEXT NOT NULL,
    lease_owner TEXT,
    lease_expires_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (tenant_id, kind, aggregate_id)
);
CREATE INDEX idx_jobs_claim ON worker_jobs(status, available_at, lease_expires_at);

CREATE TABLE outbox_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    topic TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    payload BLOB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','running','succeeded','retry','failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    available_at TEXT NOT NULL,
    lease_owner TEXT,
    lease_expires_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (tenant_id, topic, idempotency_key)
);
CREATE INDEX idx_outbox_claim ON outbox_events(status, available_at, lease_expires_at);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    result TEXT NOT NULL,
    request_id TEXT NOT NULL,
    details BLOB NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_audit_tenant_object ON audit_events(tenant_id, object_type, object_id, created_at DESC);
