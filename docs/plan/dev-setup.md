# Development setup on macOS

Run these steps at the repository root. Development state goes to `.dev/local/`, which `.git/info/exclude` covers.

1. Install the tools: `brew install podman mkcert jq`.
2. If no Podman machine exists, run `podman machine init --cpus 4 --memory 8192 --disk-size 30`.
3. Start the machine: `podman machine start`.
4. Install the mkcert root CA one time: `mkcert -install`.
5. Start the development stack: `make dev-up`. `make dev-up` creates the certificates, the keys, and the secrets. It starts `sag-dev-pg` (PostgreSQL 17 with the mkcert certificate) and `sag-dev` with `deploy/dev/sag.yaml`.
6. Issue a JWT: `export SAG_TOKEN="$(podman exec -t sag-dev sag jwt generate claude-code-dev | tr -d '\r')"`.
7. Start Claude Code with the mkcert CA: `NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" claude`.
8. Stop the stack: `make dev-down`.

Claude Code MCP configuration, user scope:

```sh
claude mcp add --transport http --scope user sag https://localhost:8443/mcp \
  --header "Authorization: Bearer ${SAG_TOKEN}"
```

Project configuration `.mcp.json` with expansion. Do not commit a literal JWT:

```json
{
  "mcpServers": {
    "sag": {
      "type": "http",
      "url": "https://localhost:8443/mcp",
      "headers": { "Authorization": "Bearer ${SAG_TOKEN}" }
    }
  }
}
```

The mkcert root CA stays on the development machine (PRD R8).
