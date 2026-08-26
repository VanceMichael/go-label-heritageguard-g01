# HeritageGuard G01

HeritageGuard is a production-oriented Go service for museum heritage-artifact custody and conservation operations. It coordinates intake, condition assessment, quarantine and treatment, exhibition-case commissioning, environmental readings and incidents, and inter-museum loan custody.

## Business paths

- Intake and conservation: register an artifact with its first condition report and custody event, record later assessments, isolate high-risk pieces, approve treatment, attach completion evidence, and release the artifact only after the treatment transaction succeeds.
- Exhibition operations: reserve a compatible case with an optimistic version, activate it only after the installation checklist is complete, accept device readings idempotently, create durable assessment jobs, and open at most one incident per threshold window.
- Loan custody: create and submit a time-bounded request, approve it against current incidents and overlapping commitments, then record packed, dispatched, returning, and returned custody events atomically with artifact state.

The service uses tenant-scoped roles (`registrar`, `conservator`, `coordinator`, and `supervisor`). Sessions are server-side, revocable, expiring, and stored as SHA-256 token hashes. An optional bootstrap supervisor is created only when the corresponding environment variables are supplied.

## Runtime

The HTTP entry point is `./cmd/server`. SQLite migrations are embedded and applied on startup. Worker jobs and outbox events use durable leases, retry backoff, and terminal failure states. `/livez` checks process liveness; `/readyz` checks the database connection.

```sh
cp .env.example .env
GOTOOLCHAIN=local go run ./cmd/server
```

For a first local account, set `HERITAGEGUARD_BOOTSTRAP_EMAIL` and a password of at least 12 characters in the environment. Do not commit `.env` or real credentials.

## Verification

```sh
GOTOOLCHAIN=local go test ./... -count=1
GOTOOLCHAIN=local go test -race ./... -count=1
GOTOOLCHAIN=local go vet ./...
GOTOOLCHAIN=local go build ./...
```

The Dockerfile builds a static `linux/arm64` image when invoked on Apple Silicon with the native Go build platform. It stores the database under `/data` and runs as the distroless non-root user.
