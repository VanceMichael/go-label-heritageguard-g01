INSERT OR IGNORE INTO tenants(id, name, timezone, created_at)
VALUES ('museum-demo', 'HeritageGuard Demonstration Museum', 'Asia/Shanghai', strftime('%Y-%m-%dT%H:%M:%fZ','now'));

INSERT OR IGNORE INTO quarantine_zones(id, tenant_id, name, capacity, occupied, version, active)
VALUES
    ('zone-textile', 'museum-demo', 'Textile Isolation Vault', 12, 0, 1, 1),
    ('zone-paper', 'museum-demo', 'Paper Isolation Vault', 16, 0, 1, 1),
    ('zone-general', 'museum-demo', 'General Isolation Vault', 20, 0, 1, 1);

INSERT OR IGNORE INTO display_cases(
    id, tenant_id, gallery, name, status, artifact_id,
    min_humidity, max_humidity, min_temp_c, max_temp_c,
    reservation_to, version, updated_at
) VALUES
    ('case-east-01', 'museum-demo', 'East Gallery', 'E-01', 'available', NULL, 45, 55, 18, 24, NULL, 1, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('case-east-02', 'museum-demo', 'East Gallery', 'E-02', 'available', NULL, 40, 60, 16, 25, NULL, 1, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('case-west-01', 'museum-demo', 'West Gallery', 'W-01', 'available', NULL, 42, 58, 17, 24, NULL, 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'));
