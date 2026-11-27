# Layer 7 gate: devx

Phase A, macOS development machine.

## D7.0 registry check

- Registry image: `docker.io/library/registry:3.0.0@sha256:832ef18332f0dde1edff9b45d796231c68f9b9108cf91acb5e6fac450b8fbdcb`.
- The container ran with `--rm`, the label `sag.devx-e2e=1`, and `-p 127.0.0.1:<free port>:5000`.
- `podman push --tls-verify=false localhost/sag:devx-e2e localhost:<port>/kanthorlabs/sag:main` exits 0.
- `podman push --digestfile` writes no digest with the remote Podman client on macOS. The digest comes from the header `Docker-Content-Digest` of `HEAD /v2/kanthorlabs/sag/manifests/main`.
- `podman pull --tls-verify=false localhost:<port>/kanthorlabs/sag@<digest>` exits 0. The same pull with TLS verification fails.
- After the check, no registry container and no pulled reference exist. No setting of `podman-machine-default` changed.
