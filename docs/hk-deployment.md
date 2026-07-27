# HK deployment

The IMLR fork publishes its `main` branch to GHCR and deploys the resulting
immutable image to the Hong Kong VPS.

## Build

`.github/workflows/container.yml` builds the existing root `Dockerfile` on
native GitHub-hosted AMD64 and ARM64 runners. Successful non-pull-request runs
publish:

- `ghcr.io/imlr/new-api:latest`
- `ghcr.io/imlr/new-api:sha-<40-character-commit>`

The VPS does not clone the repository or run a Docker build.

## Deployment

The deployment job connects as the restricted `github-deploy` SSH user. Its
key is forced to run:

```text
sudo -n /usr/local/sbin/deploy-ghcr-compose new-api
```

The server accepts only the GitHub actor, the current job's temporary
`GITHUB_TOKEN`, and a 40-character commit SHA on standard input. The token is
used through a temporary Docker configuration directory and is not stored on
the server.

Runtime configuration is stored in `/opt/new-api`:

- `compose.yaml`: copied from `deploy/hk/compose.yaml`
- `.env`: image reference and existing production secrets

The Compose project name remains `new-api-wew7hv`, so the migration reuses the
existing PostgreSQL, Redis, application data, and log volumes. Traefik reaches
the application through the external `proxy` bridge network.

After Compose reports a healthy container, the deployment script verifies
`https://aihk.imlr.dev/api/status`. A failed deployment restores the previous
immutable image.
