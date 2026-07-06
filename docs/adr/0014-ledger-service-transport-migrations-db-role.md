# ADR-0014: Ledger service — HTTP/JSON transport, embedded migrate-on-startup, dedicated DB role

- Status: Accepted
- Date: 2026-07-05
- Deciders: David Omokhodion
- Related: ADR-0003 (Ledger consistency model), ADR-0005 (Ledger datastore placement),
  ADR-0008 (Encryption via AWS-managed keys)

## Context

P2 introduces the first application code (`services/ledger`) and the first data-tier infra
(ephemeral RDS Postgres) in the repo. Three implementation choices are binding enough, and
consequential enough to future phases (`api` in P3, `worker` in P4 both call `ledger` or
share its infra patterns), to record here rather than leave as undocumented code structure.

## Decisions

### 1. Internal transport: HTTP/JSON, not gRPC

`api` → `ledger` calls travel over ECS Service Connect, which is transport-agnostic — it
provides service discovery and internal routing, not a wire format. HTTP/JSON was chosen
over gRPC:

- No `protoc`/codegen toolchain to install and maintain for a single internal caller.
- Matches the style of the public contract already defined in `API.md`, so the same
  request/response idioms carry across the public and internal surface.
- gRPC's main advantages (strict typed contracts, streaming, multiplexing) aren't needed
  for a simple internal request/response call at this scale.

Trade-off accepted: no compile-time contract checking between `api` and `ledger` — mitigated
by both living in the same Go module, so shared request/response types are just imported, not
duplicated or generated.

### 2. Migrations: embedded golang-migrate, run on startup

The `ledger` binary embeds its `.sql` migrations (via golang-migrate) and applies any pending
ones automatically before it starts accepting traffic, rather than running migrations as a
separate CI/CD step or one-off ECS task.

- Directly satisfies ADR-0005's ephemeral-lifecycle requirement: "migrations and seed run
  automatically on every bring-up" — apply the Terraform, start the task, and the schema (and
  seed data) exist with no extra manual or pipeline step.
- The system-account seed is itself an idempotent (`ON CONFLICT DO NOTHING`) migration, so
  repeated bring-up/teardown cycles stay reproducible.

Trade-off accepted: with more than one task replica, multiple instances could race to apply
migrations on cold start. Accepted for now — `golang-migrate`'s advisory lock around the
migration run serializes this safely, and P2 runs a single replica.

### 3. DB credentials: dedicated least-privilege role, not the RDS master user

The RDS instance gets an auto-generated master password (Secrets Manager, never touched by
application code). A separate Terraform `postgresql` provider block — authenticating with
that master secret at apply time only — provisions a dedicated `ledger` database and role
scoped to just that schema. The ledger ECS task is injected with *that* role's credentials,
via its own Secrets Manager secret.

- Matches `CLAUDE.md`'s least-privilege rule, which otherwise applies mainly to IAM; a money
  ledger's database access deserves the same discipline.
- No service ever holds RDS master/superuser-equivalent credentials.

Trade-off accepted: one more Terraform provider wired into `infra/live/dev/rds` (the
`postgresql` provider pointed at the freshly-created RDS endpoint) versus simply reusing the
master credentials — a small amount of extra apply-time complexity for a real security
improvement on the money-movement path.

## Consequences

Positive: reproducible ephemeral bring-up with no manual migration/seed step; a defensible
least-privilege story for the database layer, not just IAM; no new toolchain (protoc)
required for internal service-to-service calls.

Negative: no compile-time contract enforcement between `api` and `ledger` (mitigated by
shared Go types); migration-on-startup requires care if replica count grows past 1 in a
later phase (out of scope for P2's single-replica ledger).

## Alternatives considered

- **gRPC internal transport**: rejected for now — see above; revisit if replicated services
  need stronger typed contracts or streaming.
- **Separate migration step (CI job or one-off ECS task)**: rejected as unnecessary machinery
  at this scale; embedded migrate-on-startup already satisfies ADR-0005's reproducibility bar.
- **RDS master user for the app**: rejected — violates least privilege for a money ledger's
  data tier.
