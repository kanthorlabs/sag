# Testing

## 1. Test tiers

| Tier | Build tag | Needs | Command |
|---|---|---|---|
| Unit | none | Go only | `make test` (`go vet ./...` and `go test -count=1 ./...`) |
| Integration | `integration` | Podman, one PostgreSQL container | `make test-integration PG=17 PKG=<pattern> RUN=<regexp>` |
| e2e | `e2e` | Podman, SAG images, PostgreSQL containers | `make e2e LAYER=<N> PG=17 [RUN=<regexp>]` |

Test names: unit and integration tests that encode an AC start with `TestAC<NN>_`. e2e subtests are `AC<NN>_<Name>`.

## 2. PostgreSQL in Podman

`internal/testenv` is the only code that starts PostgreSQL for tests. The integration tier and the e2e tier both use it.

- Each run creates a Podman network `sag-e2e-<run>` and containers named `sag-e2e-<run>-pg<major>`.
- `testenv` generates a random superuser password with `crypto/rand`. It stores the password in a Podman secret `pg-password-<run>`. The container reads it through `POSTGRES_PASSWORD_FILE`. The password is never in a container environment variable.
- `testenv` generates a test CA and a server certificate with `crypto/x509`. The certificate SAN lists the container name and `localhost`. The key goes to a Podman secret mounted with `uid=999,gid=999,mode=0600`.
- Server flags: `-c ssl=on -c ssl_cert_file=... -c ssl_key_file=... -c log_statement=all`.
- The port publishes on `127.0.0.1` with a random host port.
- SAG connects to PostgreSQL as `postgres`, a superuser, through the container name. The container name is not a loopback IP literal, so TLS `verify-full` applies with the test CA as `tls_ca`.
- Fixtures connect as `postgres` through the published port. Fixtures use single autocommit statements only. They never send `BEGIN` or `COMMIT`, because AC8 counts both.
- Each run also starts `postgres:13-bookworm` for AC9.
- Server log: `podman logs --since <run start> <container>`.
- Teardown removes the containers, the network, and the secrets of the run. `SAG_E2E_KEEP=1` keeps them for debug.

Major selection: `SAG_TEST_PG_MAJOR` (default 17). Allowed values: 14 to 19.

## 3. e2e harness

Definition of a full e2e test:

- The real `sag` image runs in Podman. Credentials, the master key, and the TLS files come from Podman secrets.
- The test client calls MCP Streamable HTTP over TLS with a real JWT.
- The target is a real PostgreSQL container. SAG connects as `postgres`, a superuser.
- An operator action runs through `podman exec -it` on a pseudo-terminal that the test drives.

AC registry:

- Each AC has one file `e2e/ac<NN>_test.go`. One task owns it.
- The file registers one entry in `init()` with `harness.Register(harness.AC{...})`. The entry holds `ID`, `Phase`, `Agents`, `Resources` (fragments of `sag.yaml`), `Fixture` (SQL that runs before phase 1), and `Run`.
- The harness composes one `sag.yaml` from all fragments. Each AC uses its own agent `sub`, for example `ac05`. This isolates the semaphore and filters the audit lines.
- `e2e/acs.go` holds the 35 rows of the AC table: ID, owner layer, subtest name.

Containers:

- `make e2e` builds `localhost/sag:e2e` from `deploy/Containerfile` and, from layer 2, `localhost/sag-conformance:e2e` from `e2e/conformance/Containerfile`.
- The harness starts each SAG container with the flags in `e2e/harness/flags.go`: `--read-only`, `--cap-drop=all`, `--security-opt=no-new-privileges`, `--user 65532:65532`, `-p 127.0.0.1:<port>:8443`, a config bind mount at `/etc/sag/sag.yaml` (read-only), a state volume at `/var/lib/sag`, and one `--secret <name>,type=mount,uid=65532,mode=0400` for each secret. `deploy/sag.container` holds the same values (`T6.1` tests this).
- SAG containers in one run: `sag` (product, all resources), `sag-conformance` (AC5, AC6), `sag-v2` (AC3, `auth.version` change), `sag-ro-audit` (AC27), `sag-rate` (AC23, `max_concurrent_per_agent: 4`, resources `rate-a` and `rate-b`), and short-lived containers for AC1 and AC2.
- The harness creates the master key, so it can mint invalid JWTs for AC3. It gets valid JWTs from `podman exec -it <sag> sag jwt generate <sub>` on a pseudo-terminal.
- The MCP client is the go-sdk client with `StreamableClientTransport`. Its HTTP client trusts the test CA and adds `Authorization: Bearer <JWT>`.
- The harness reads the audit directory with `podman cp <sag>:/var/lib/sag/audit <tmp>`.
- For AC33 only, the harness mounts a static busybox binary read-only at `/e2e/busybox`. The production image and the Quadlet file have no shell and no busybox.

Structure of the e2e test binary:

- One top-level test `TestE2E` in `e2e/e2e_test.go`.
- Phase 1 runs `t.Run("parallel", ...)` with each phase-1 AC subtest marked `t.Parallel()`.
- Phase 2 runs sequentially after phase 1, in this order: `AC16_LexerSettings`, `AC08_RollbackOnly`, `AC19_Integrity`.
- Subtest name form: `TestE2E/<phase>/AC<NN>_<Name>`.
- Run timeout: 30 minutes.

## 4. AC report

Command: `make e2e LAYER=<N> PG=<major>`. It runs:

1. `make test`.
2. The image builds.
3. `go test -tags e2e -json -count=1 -timeout 30m ./e2e/... > e2e/report/pg<major>/events.json`.
4. `go run ./e2e/cmd/acreport -layer <N> -in e2e/report/pg<major>/events.json -out e2e/report`.

`make e2e-matrix LAYER=<N> PGS="14 17 19"` runs step 3 for each major in parallel and merges the results. `e2e/report/` is not committed.

Status rules:

| Status | Rule |
|---|---|
| `PASS` | The subtest passed on every major in the run. |
| `FAIL` | The subtest failed or timed out on one major or more, or a prerequisite is missing (for example no `claude` login for AC31). |
| `NOT_IMPLEMENTED` | No subtest for the AC ran. |

`e2e/report/ac-report.json`:

```json
{
  "schema": 1,
  "generated_at": "2026-11-25T12:00:00.000Z",
  "commit": "<git rev-parse HEAD>",
  "dirty": true,
  "layer": 2,
  "image": "<sag image digest>",
  "pg_majors": [14, 17, 19],
  "acs": [
    {
      "id": "AC05",
      "layer": 2,
      "test": "TestE2E/parallel/AC05_C2BlocksWrites",
      "status": "PASS",
      "duration_ms": 4120,
      "by_major": { "14": "PASS", "17": "PASS", "19": "PASS" },
      "message": ""
    }
  ],
  "gate": { "layer": 2, "passed": true, "failing": [] }
}
```

`e2e/report/ac-report.md` holds one table with 35 rows: `AC`, `Layer`, `Test`, `Status`, one column per major, `Duration`, `Message`. The header lists the commit, the image digest, and the gate result.

Gate rule: `acreport` exits 1 if any AC with `layer <= N` is not `PASS`, or if any other subtest of the run fails (for example `Harness_Skeleton`). It lists each failing test.

## 5. CI (AC35)

`.github/workflows/ci.yml` on GitHub Actions:

| Job | Runner | Steps | Task |
|---|---|---|---|
| `unit` | `ubuntu-24.04` | `make test` | `X.1` |
| `build-macos` | `macos-14` (arm64) | `go build ./...` | `X.1` |
| `e2e` | `ubuntu-24.04`, matrix `pg: [14, 15, 16, 17, 18, 19]` | `make e2e LAYER=6 PG=${{ matrix.pg }} AC_SKIP=AC31`, upload `e2e/report/pg<major>/` | `T6.3` |
| `ac35` | `ubuntu-24.04`, needs all jobs above | `go run ./e2e/cmd/acreport merge`: AC35 is `PASS` only if every matrix report passes every AC except AC31, and `build-macos` passes | `T6.3` |

- The runner user is not root. AC27 needs this, because root ignores directory modes.
- Pin each action by commit SHA.
- Runner Podman is rootless.

## 6. AC map

| AC | Layer | e2e subtest | Owner task | Automation |
|---|---|---|---|---|
| AC1 | 1 | `AC01_ConfigCheckRejects` | `T1.1` | Full. Five fixtures; the valid file runs without `/run/secrets`. |
| AC2 | 1 | `AC02_StartRefusesBadSecrets` | `T1.2` | e2e without an MCP call: the container with bad secrets exits non-zero. |
| AC3 | 1 | `AC03_JWTRejects` | `T1.3` | Full |
| AC4 | 1 | `AC04_JWTGenerateRefuses` | `T1.3` | Full, pseudo-terminal |
| AC5 | 2 | `AC05_C2BlocksWrites` | `T2.7` | Full on `sag-conformance` |
| AC6 | 2 | `AC06_SingleStatement` | `T2.7` | Full on `sag-conformance` |
| AC7 | 2 | `AC07_NoPassThrough` | `T2.7` | e2e without an MCP call: `go list -deps ./cmd/sag` and the symbol table of the `sag` binary from `localhost/sag:e2e`. |
| AC8 | 2 | `AC08_RollbackOnly` | `T2.1` | Full, phase 2 |
| AC9 | 2 | `AC09_RefusesPG13` | `T2.3` | Full |
| AC10 | 2 | `AC10_TLSVerifyFull` | `T2.3` | Full |
| AC11 | 2 | `AC11_ExtensionApproval` | `T2.4` | Full, pseudo-terminal, `preflight.recheck_interval: 10s` |
| AC12 | 2 | `AC12_OperatorCollision` | `T2.3` | Full |
| AC13 | 3 | `AC13_StaticRejects` | `T3.2` | Full |
| AC14 | 3 | `AC14_FieldAllowlist` | `T3.1` | Full through a field case. The hand-built tree fixture is the unit test. |
| AC15 | 3 | `AC15_C1OnlyEffects` | `T3.4` | Full |
| AC16 | 3 | `AC16_LexerSettings` | `T3.5` | Full, phase 2 |
| AC17 | 3 | `AC17_RenderedTextOnly` | `T3.5` | Full |
| AC18 | 3 | `AC18_ViewWriteBlocked` | `T3.6` | Full |
| AC19 | 3 | `AC19_Integrity` | `T3.6` | Full, phase 2, last |
| AC20 | 4 | `AC20_MaxRows` | `T4.1` | Full |
| AC21 | 4 | `AC21_MaxResultBytes` | `T4.2` | Full |
| AC22 | 4 | `AC22_LockTimeout` | `T4.3` | Full |
| AC23 | 4 | `AC23_RateLimit` | `T4.1` | Full on `sag-rate`: 2 requests on each of 2 resources, the fifth gets `rate_limited` |
| AC24 | 4 | `AC24_ConnectErrorScrub` | `T4.3` | Full |
| AC25 | 4 | `AC25_ColumnRedaction` | `T4.2` | Full |
| AC26 | 4 | `AC26_AuditRecord` | `T4.4` | Full |
| AC27 | 4 | `AC27_AuditFailure` | `T4.4` | Full. Needs a non-root test user. |
| AC28 | 5 | `AC28_EndpointAuthOrigin` | `T5.1` | Full |
| AC29 | 5 | `AC29_Authorization` | `T5.2` | Full |
| AC30 | 5 | `AC30_ToolShapes` | `T5.2` | Full |
| AC31 | 5 | `AC31_ClaudeCode` | `T5.3` | Local only. Needs a logged-in Claude Code and network access to Anthropic. CI skips it. |
| AC32 | 6 | `AC32_ContainerInspect` | `T6.1` | Full |
| AC33 | 6 | `AC33_SecretMounts` | `T6.2` | Full, with the e2e busybox mount |
| AC34 | 6 | `AC34_JWTGenerateNoTTY` | `T6.1` | Full |
| AC35 | 6 | CI job `ac35` | `T6.3` | CI only. Needs a push of the branch by Ulrich. |
