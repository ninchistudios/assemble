# Deployment

How to run `assemble` for real, on a public domain. Start with the [README quick start](../README.md#quick-start). It covers the Google OAuth client and the environment variables, which are the same here.

Everything in this file is generic. Keep your real domain, client secret and admin email in `.env` and `docker-compose.yml`, which are both gitignored, and never in this repo or its issues.

## What you need

- A Docker host with a public DNS name pointing at it.
- A reverse proxy that terminates TLS. The examples use Nginx Proxy Manager (NPM), but any proxy works.
- A Google OAuth client whose redirect URI is `https://YOUR-DOMAIN/auth/callback`, with the consent screen **In production** (README step 1).

## The image

GitHub Actions publishes `ghcr.io/ninchistudios/assemble` with two kinds of tag:

| Tag | Tracks |
|---|---|
| `main` | The latest commit on `main` |
| `2026.2` | The latest commit on a release branch, while it is being tested before merging |
| `v2026.2` | A release, frozen |

For the low-maintenance setup this app is built for, pin a release tag and move it on deliberately. Use `main` only if you want every change as it lands.

If the package is private, the host needs `docker login ghcr.io` with a token that can read packages. Making the package public in its GitHub settings means no credential on the host that could expire.

## Configure

Use the variables from the README, with your public origin:

```sh
ASSEMBLE_BASE_URL=https://assemble.example.com
ASSEMBLE_GOOGLE_CLIENT_ID=...
ASSEMBLE_GOOGLE_CLIENT_SECRET=...
ASSEMBLE_ADMIN_EMAIL=you@example.com
```

`ASSEMBLE_BASE_URL` must be the real public origin. TLS ends at the proxy, so the container only ever sees plain HTTP. `assemble` therefore takes the cookie `Secure` flag from the base URL, not from the request, and the base URL is also what `POST` requests are checked against (their `Origin` header must match).

Leave `ASSEMBLE_DEV_BYPASS` unset. It is for `go run` on a developer machine only: the app refuses to start with it unless the base URL is `http://localhost` and it listens on a loopback address, which rules out running it in a container.

### Docker Compose

The example compose file has the important parts already:
- the database on the named volume `assemble-data`
- `ASSEMBLE_DB_PATH=/data/assemble.db`
- a healthcheck
- no published port
- the container restrictions and log rotation suggested in the [README's Hardening section](../README.md#hardening)

```sh
docker compose up -d
docker compose logs -f assemble
```

### Portainer

Stacks written in Portainer's web editor **ignore `env_file:`**, because there's no `.env` file beside the compose file. Instead:

- Replace `env_file: .env` with an `environment:` block that uses `${VARIABLE}` references.
- Set the values in the stack's **Environment variables** panel.

```yaml
    environment:
      ASSEMBLE_BASE_URL: ${ASSEMBLE_BASE_URL}
      ASSEMBLE_GOOGLE_CLIENT_ID: ${ASSEMBLE_GOOGLE_CLIENT_ID}
      ASSEMBLE_GOOGLE_CLIENT_SECRET: ${ASSEMBLE_GOOGLE_CLIENT_SECRET}
      ASSEMBLE_ADMIN_EMAIL: ${ASSEMBLE_ADMIN_EMAIL}
      ASSEMBLE_DB_PATH: /data/assemble.db
```

When you update the stack, tick **Re-pull image**. Without it, a moving tag like `main` never changes.

## Reverse proxy

The app always listens on **8080 inside the container**. Prefer publishing no host port at all: put `assemble` and the proxy on a shared Docker network and let the proxy reach the container by name. Then nothing is exposed on the host and there is no way around the proxy.

For NPM, add a Proxy Host:

- **Domain Names:** your domain
- **Scheme:** `http`. The container speaks plain HTTP; choosing `https` here gives a 502.
- **Forward Hostname / IP:** `assemble` (the container name) when NPM runs in Docker on the same network. Use `127.0.0.1` if NPM runs on the host.
- **Forward Port:** `8080` on a shared network. If NPM runs on the host, use the published host port: publish `127.0.0.1:8138:8080`, never `0.0.0.0`.
- **Block Common Exploits:** on
- **SSL:** request a Let's Encrypt certificate, then enable **Force SSL** and **HTTP/2**. HSTS is safe to enable.

To check, open `https://YOUR-DOMAIN/healthz`. It should return `ok <version> build <branch>:<commit>`, the same version the page footer shows.

## First sign-in

1. Sign in with the `ASSEMBLE_ADMIN_EMAIL` account. It becomes the admin and is tied to that Google account from then on.
   - If a different Google account later presents the same address (because the address was reassigned), it is refused.
   - To hand the admin role to someone else, set `ASSEMBLE_ADMIN_EMAIL` to their address and restart. They become admin at their next sign-in, and the role is tied to their Google account from then on. The previous admin keeps the role.
2. Set the group's name, timeslot and timezone under **Admin → Group**.
3. Create an invite link under **Admin → Invites** and post it to the group. Once everyone has joined, disable it.

Then walk the [test plan](testplan.md).

## Backups

Everything lives in one SQLite database on the `assemble-data` volume (`/data/assemble.db`, plus its `-wal` and `-shm` files while running). The image is distroless, with no shell or `sqlite3`, so back it up from a throwaway container.

**Online backup:** a consistent copy, taken while the app runs.

```sh
docker run --rm --volumes-from assemble -v "$PWD":/backup alpine \
  sh -c 'apk add -q sqlite && sqlite3 /data/assemble.db ".backup /backup/assemble-$(date +%F).db"'
```

**Offline backup:** stop the container and copy the whole volume.

```sh
docker compose stop assemble
docker run --rm --volumes-from assemble -v "$PWD":/backup alpine \
  tar czf /backup/assemble-data-$(date +%F).tgz -C /data .
docker compose start assemble
```

**Restore:** stop the container, put the backup file in place as `/data/assemble.db` (removing any `-wal` and `-shm` files), and start it again.

`--volumes-from assemble` mounts exactly the volume the `assemble` container uses, whatever Compose or Portainer named it. It works whether that container is running or stopped.

## Updating

```sh
docker compose pull && docker compose up -d
```

Database migrations run automatically at startup. Sessions live in the database, so updates never sign anyone out.

**Back up before moving to a new release.** Migrations only go forward. If an older image starts against a database that a newer one has already migrated, it refuses to run (`database is at schema N but this build only knows M`) rather than damage it. To roll back, restore the backup taken before the upgrade.

## Building the image yourself

`.dockerignore` excludes `.git`, so the build can't read the revision or branch itself. Pass them in, or the footer shows the version alone:

```sh
docker build \
  --build-arg VERSION="$(cat VERSION)" \
  --build-arg COMMIT="$(git rev-parse HEAD)$(test -z "$(git status --porcelain)" || echo +)" \
  --build-arg BRANCH="$(git branch --show-current)" \
  -t assemble:local .
```

The `VERSION` file at the repo root is the app version. A trailing `+` on the commit marks a build made from uncommitted changes. The footer and `/healthz` show the branch and the first 8 characters of the commit, e.g. `2026.1 build main:156fac22`. CI images carry the branch or tag they were built from.

To build with Compose instead, replace `image:` with a `build:` section that passes them through:

```yaml
    build:
      context: .
      args:
        VERSION: "2026.2"   # match the VERSION file
        COMMIT: ${ASSEMBLE_COMMIT:-}
        BRANCH: ${ASSEMBLE_BRANCH:-}
```

```sh
# bash
ASSEMBLE_COMMIT=$(git rev-parse HEAD) ASSEMBLE_BRANCH=$(git branch --show-current) docker compose up -d --build
# PowerShell
$env:ASSEMBLE_COMMIT = git rev-parse HEAD; $env:ASSEMBLE_BRANCH = git branch --show-current; docker compose up -d --build
```

A copy whose `ASSEMBLE_BASE_URL` is `localhost` or `127.0.0.1` marks every page with a muted red header and a **local** tag, so a test copy is never mistaken for the real one.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `redirect_uri_mismatch` at Google | `ASSEMBLE_BASE_URL` + `/auth/callback` doesn't exactly match a redirect URI on the OAuth client. Scheme, host and port must all match. |
| Members get "access blocked" or an error at Google, but the admin can sign in | The consent screen is still in **Testing**. Publish it to **In production** (README step 1). |
| Brand verification: "not registered to you" | The homepage's domain isn't verified in Search Console by the account that owns the Cloud project (README step 1). Verify it, then wait 24 hours before retrying. |
| Brand verification: app name "does not match" | The consent screen's app name must match the group name shown on the homepage. |
| Container exits with `config: missing required environment: ...` | The message names every variable that is unset. On Portainer, remember that `env_file:` is ignored. |
| `ASSEMBLE_DEV_BYPASS only runs against http://localhost`, or `needs ASSEMBLE_LISTEN on a loopback address` | The dev bypass was left on in a server config. Remove it. |
| 502 from the proxy | The proxy scheme is `https`; it must be `http`. Or the proxy and container aren't on the same Docker network. |
| "That request didn't come from this site." on every form | `ASSEMBLE_BASE_URL` doesn't match the address in the browser, e.g. `http` vs `https`, or `www.` vs bare domain. |
| "Not a member" for the admin | The address in `ASSEMBLE_ADMIN_EMAIL` doesn't match the Google account, or it already belongs to another account (see First sign-in). |
| "Sign-in expired. Please try again." | The 10-minute sign-in window lapsed, or the tab was stale. Harmless; retry. |
| `/healthz` returns 503 | The database can't be reached. Check the volume is mounted and `ASSEMBLE_DB_PATH` points into it. |
