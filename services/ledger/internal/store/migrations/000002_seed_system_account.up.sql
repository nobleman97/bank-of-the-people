-- Idempotent: safe to re-run on every bring-up (ADR-0005).
INSERT INTO accounts (id, currency)
VALUES ('00000000-0000-0000-0000-000000000001', 'USD')
ON CONFLICT (id) DO NOTHING;
