# Layer 8 gate: devx

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
