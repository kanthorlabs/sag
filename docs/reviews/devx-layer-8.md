# Layer 8 gate: devx

Phase A, macOS development machine. Gate commit `500cf0c`.

## D8.0 checks

### LocalStack image

Image: `docker.io/localstack/localstack@sha256:b279c01f4cfb8f985a482e4014cabc1e2697b9d7a6c8c8db2e40f4d9f93687c7`

Tag: `localstack/localstack:3.8` (version 3.8.1)

License: Apache-2.0 (community edition). No auth token required. No LOCALSTACK_AUTH_TOKEN needed to start or to serve secretsmanager.

The community edition reports `"edition": "community"` in `/_localstack/health`.

The container started with `--rm`, `--label sag.devx-e2e=1`, `-p 127.0.0.1:4566:4566`, and no LOCALSTACK_AUTH_TOKEN set. The `secretsmanager` service reported `"available"` within 8 seconds of container start.

### Secrets Manager operations confirmed

All four required operations passed against `http://127.0.0.1:4566` with AWS credentials `test`/`test` and region `us-east-1`:

- `CreateSecret`: created `sag/test/conn` with tag `Key=sag.resource,Value=test`. Exit 0.
- `GetSecretValue`: returned `SecretString` matching the value stored. Exit 0.
- `ListSecrets` with `--filter Key=tag-key,Values=sag.resource`: returned the created secret. Exit 0.
- `DeleteSecret` with `--force-delete-without-recovery`: removed the secret immediately. Exit 0.

### Packet drop method

Method: `podman pause <container>` and `podman unpause <container>`.

The method does not change any setting of `podman-machine-default`.

Result of `podman pause`: a `curl --max-time 5` call to the LocalStack endpoint timed out after 5.035 seconds. `curl` exit code was 28 (operation timed out). The client received no connection refused error; the connection hung until the timeout.

Result of `podman unpause`: the endpoint answered within 2 seconds. The `secretsmanager` service reported `running`.

The test sets `podman pause` before the AWS client call and `podman unpause` after the timeout check.

## Tasks

| Task | Merge |
|---|---|
| `D8.0` contract and LocalStack check | `c2eb97d` |
| `D8.1` `aws-sm:` reference, `storage.aws`, AWS backend | `bd1a6d3` |
| `D8.2` daemon availability without AWS (DX-AC32) | `52e8bd5` |
| `D8.3` AWS backend for the credential commands | `52e8bd5` |
| `D8.4` integration: AWS endpoint hook, DX-AC31 clauses | `52e8bd5` |
| `D8.5` review fixes | `500cf0c` |

## Decisions

- D8-b: the test build reads `SAG_TEST_AWS_ENDPOINT` and gives `AWS_ENDPOINT_URL_SECRETS_MANAGER` to the container. The product build has no such path.
- D8-c: a resource with an `aws-sm:` reference in `connection` or `tls_ca` is unopened at start. The background open loads each `aws-sm:` reference. `sag resource test` uses the same load.
- D8-d: the daemon builds the AWS backend only for an `aws-sm:` reference. A build error makes only the AWS resources unavailable. `--backend` takes `podman` or `aws-sm`.

## Findings fixed at integration

- B118 - status:FIXED - action:YES - an `aws-sm:` TLS CA was never loaded - fix:load each `aws-sm:` reference of a resource - why:PRD-devx 11.2 lets a resource mix backends
- B119 - status:FIXED - action:YES - DX e2e used the host `aws` CLI and passed each `AWS_*` variable of the operator - fix:SDK client with static test keys; exact allow list - why:no test reads `~/.aws` or real credentials
- B120 - status:FIXED - action:YES - the DXAC32 state check could not fail - fix:three states: served, `resource_unavailable`, other - why:a check that cannot fail does not count
- B121 - status:FIXED - action:YES - weak DXAC31 checks - fix:exact create line, exact tags and absence of each secret - why:DX-AC31 says each created AWS secret
- B122 - status:FIXED - action:YES - the name test did not call `ParseRef` - fix:test `ParseRef` - why:the product path

## Independent review (Fable)

The review covered `sag-devx-7..c931f60`. Task `D8.5` fixed the items.

- B123 - status:FIXED - action:YES - `--backend podman` rejected - fix:`podman|aws-sm`, default `podman` - why:PRD-devx 7.1 and 7.5
- B124 - status:FIXED - action:YES - delete compared `aws-sm:` references with bare AWS names - fix:compare with the prefix - why:a false `skip` line, and a cross-referenced AWS secret was deleted
- B125 - status:FIXED - action:YES - AWS `List` read one page - fix:follow `NextToken` - why:PRD-devx 11.1
- B126 - status:FIXED - action:YES - the AWS backend build could stop the daemon - fix:build only for an `aws-sm:` reference; a build error is not fatal - why:PRD-devx 11.4
- B127 - status:FIXED - action:YES - an AWS-loaded `tls_ca` did not reach the scrubber - fix:add each loaded value - why:PRD-devx 11.4
- B128 - status:FIXED - action:YES - DXAC31 checked less than the AC text - fix:CA secret, `exists` and `owned`, no `skip` line - why:DX-AC31
- S86 to S88, S91, S92 - status:FIXED - action:YES - separate AWS key values in the scrubber, scrubber test on daemon code, create rule order, exact unreachable text, injectable AWS constructor
- S89 - status:OPEN - action:NO - the AWS open waits for the first Podman preflights - fix:none - why:the DX-AC32 limits hold
- S90 - status:OPEN - action:NO - the `aws-sm:` prefix check is in four places - fix:none - why:behavior is correct

## Gate 1 on `52e8bd5`

All checks exit 0 except `matrix` exit 2. The cause is B129.

- B129 - status:FIXED - action:YES - `TestExpectErrorRedacts` lost the PTY output of a child that exited at once - fix:the child waits for one input line - why:a harness test must not depend on scheduler timing

## Final gate on `500cf0c`

```
gofmt -l internal e2e cmd                    → empty
go vet ./... and -tags devxe2e               → exit 0
go test -tags devxtest ./internal/...        → exit 0
make contract-check                          → exit 0
make test                                    → exit 0
make e2e-devx-unit LAYER=8                   → exit 0
make e2e-devx SUITE=Main LAYER=8             → two runs, 140 pass events each, no fail, no skip
make e2e-devx SUITE=Pins LAYER=8             → exit 0, report over Main and Pins at 500cf0c
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0
```

The two Main runs exit 2 only because the report of the earlier Pins run named `52e8bd5`. The Pins run then wrote the report over both suites at `500cf0c`.

```
PASS DX_Harness_Skeleton  PASS DXAC01  PASS DXAC02  PASS DXAC03  PASS DXAC04
PASS DXAC06  PASS DXAC07  PASS DXAC08  PASS DXAC09  PASS DXAC10
PASS DXAC11  PASS DXAC12  PASS DXAC13  PASS DXAC14  PASS DXAC15
PASS DXAC16  PASS DXAC17  PASS DXAC18  PASS DXAC19  PASS DXAC20
PASS DXAC21  PASS DXAC22  PASS DXAC23  PASS DXAC24  PASS DXAC25
PASS DXAC26  PASS DXAC27  PASS DXAC28  PASS DXAC29  PASS DXAC30
PASS DXAC31  PASS DXAC32  PASS DXAC33  PASS DXAC36  PASS DXAC37
PASS DXAC39  PASS DXAC40  PASS DXAC41  PASS DXAC42  PASS DXAC43
PASS DXAC44  PASS DXAC45  PASS DXAC46  PASS DXAC49  PASS DXAC50
PASS DXAC51  PASS DXAC53
```

DXAC31 and DXAC32 alone also passed two times on `c931f60` and two times on `1ac1be2`. After the runs, `~/.config/sag`, the LaunchAgent, each container with label `sag.devx-e2e=1`, the LocalStack container, the secret `sag-aws-credentials` and each AWS test secret are absent.

## Disk

```
podman system df before and after the gate: equal
Images 98, reclaimable 11.84GB; Local Volumes 17, reclaimable 823.8MB
/var free after the gate: 11725840 KiB
```

The gates and the final DXAC31 and DXAC32 runs added no Podman volume. Earlier failed task runs left three unlabeled anonymous volumes: one LocalStack data volume and two PostgreSQL data volumes. They wait for a manual removal (S94).

## Open items

- S89, S90 - status:OPEN - action:NO - see above.
- S94 - status:OPEN - action:YES - three unlabeled orphan volumes from earlier task runs - fix:manual removal by the operator - why:the safety rules allow removal only of labeled objects
- S93 - status:OPEN - action:NO - reclaimable image size grew by the pinned LocalStack image (1.34 GB) - fix:none - why:a pinned test image, not a leak
