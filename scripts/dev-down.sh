#!/usr/bin/env bash
set -euo pipefail

podman rm --force --ignore --volumes --time 0 sag-dev sag-dev-pg >/dev/null

if podman volume exists sag-dev-state; then
  podman volume rm sag-dev-state >/dev/null
fi

if podman network exists sag-dev; then
  podman network rm sag-dev >/dev/null
fi

for name in sag-dev-tls-cert sag-dev-tls-key sag-dev-master-key sag-dev-orders sag-dev-pg-ca sag-dev-pg-password sag-dev-pg-cert sag-dev-pg-key; do
  if podman secret exists "$name"; then
    podman secret rm "$name" >/dev/null
  fi
done

if podman image exists localhost/sag-dev:latest; then
  podman rmi localhost/sag-dev:latest >/dev/null
fi
