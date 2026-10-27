# assemble

Find the days your group can get together.

`assemble` is a small, self-hosted web app for groups that need to line up everyone's free evenings: a band booking practice, a D&D table, a book club. Each member marks every day as **YES** (preferred), **OK** (possible) or **NO**, and the admin locks in a day once everyone can make it.

Each Docker container runs one group, which signs in with Google and joins through an invite link. It is designed for phones first and to run for months without attention.

## Status

**2026.2** is the first release a group can use day to day:
- **Members** open the app on today, set their availability with one tap, swipe between days, and leave a short note.
- **Everyone** sees each member's answer per day, plus an overview of the next two months.
- **The admin** locks in a day. A member pulling out cancels it, after a confirm, and everyone gets a notice either way.
- **Also:** members choose the name the group sees them by; the admin can keep a language filter for names and notes; and there are public privacy and terms pages for Google's consent screen.

**2026.1** laid the foundation: Google sign-in, invite links, member management and group settings.

See [agents.md](agents.md) for the product design and [docs/calendar.md](docs/calendar.md) for how the calendar works.

## Quick start

This runs a private instance on your own machine at `http://localhost:8138`, signing in with a real Google account. You need Docker and a Google account. For a public deployment, read this first, then [docs/deployment.md](docs/deployment.md).

### 1. Create a Google OAuth client

`assemble` uses Google only to learn who is signing in. It asks for your name and email, and nothing else.

1. Open the [Google Cloud console](https://console.cloud.google.com/) and create a project. Any name will do, e.g. `assemble`.
2. Go to **APIs & Services → OAuth consent screen**. On newer consoles this is labelled **Google Auth Platform**. Choose **Get started**, then:
   - **App information:** set the app name to your group's name exactly as `assemble` shows it (the admin's Group setting), and your email as the support email. Google's brand check compares this name with your homepage.
   - **Audience:** choose **External**.
   - **Contact information:** your email.
   - Agree to the policy and create.
3. **Branding:** under **App domain**, set the privacy policy link to `https://YOUR-DOMAIN/privacy` and the terms of service link to `https://YOUR-DOMAIN/terms`. `assemble` serves both pages without sign-in. They describe what the software stores; if you need wording specific to your group, take your own advice on it.
   - Add your domain under **Authorised domains**. Google also requires the domain to be verified in [Search Console](https://search.google.com/search-console) by the Google account that owns this Cloud project. A **Domain** property verified by a DNS TXT record is the simplest route, and a domain can have several verified owners.
4. **Audience → Publish app**, so that the publishing status reads **In production**.
   - This step matters. While the app is in **Testing**, only Google accounts you list by hand can sign in, so invite links fail for everyone else.
   - `assemble` asks only for the basic `openid`, `email` and `profile` scopes, so publishing needs no review from Google.
5. **Clients → Create client** (older consoles: **Credentials → Create credentials → OAuth client ID**):
   - **Application type:** Web application
   - **Authorised redirect URIs:** `http://localhost:8138/auth/callback`
   - Add your public one too, if you'll deploy (e.g. `https://assemble.example.com/auth/callback`). One client can hold both.
6. Copy the **client ID** and **client secret**. Newer consoles show the secret only once, so save it now.

The redirect URI must match `ASSEMBLE_BASE_URL` plus `/auth/callback` exactly, including scheme and port. Google accepts `http` only for `localhost` and `127.0.0.1`. A LAN address like `http://192.168.1.20:8138` will be refused, so test from the machine running Docker.

### 2. Configure

```sh
cp .env.example .env
cp docker-compose.example.yml docker-compose.yml
```

In `.env`, set:

```sh
ASSEMBLE_BASE_URL=http://localhost:8138
ASSEMBLE_GOOGLE_CLIENT_ID=<from step 1>
ASSEMBLE_GOOGLE_CLIENT_SECRET=<from step 1>
ASSEMBLE_ADMIN_EMAIL=<your Google address>
```

In `docker-compose.yml`, uncomment the `ports:` lines so the app is reachable on `127.0.0.1:8138`. If you're building from source rather than pulling the image, replace `image:` with `build: .`.

Both files are gitignored. Never commit real values.

### 3. Run

```sh
docker compose up -d
docker compose logs -f assemble
```

The log should say `assemble <version> build <branch>:<commit> listening on :8080 for http://localhost:8138`. Port 8080 is inside the container; you browse to 8138.

### 4. Sign in and invite people

1. Open `http://localhost:8138` and choose **Sign in with Google** with the account in `ASSEMBLE_ADMIN_EMAIL`. That account becomes the admin.
2. **Admin → Group:** set the group name, the daily timeslot and the timezone, e.g. `Australia/Brisbane`.
3. **Admin → Invites → Create invite link**, and send it to the group. Anyone who opens it can join with their Google account.
4. Once everyone has joined, **disable** the link. **Admin → Members** is where you disable or re-enable anyone.

Anyone else who signs in is refused: a Google account that is neither the admin nor came through an active invite link.

## Configuration

All configuration is environment variables. Group settings (name, timeslot, timezone) are set in the app and stored in the database.

| Variable | Required | Notes |
|---|---|---|
| `ASSEMBLE_BASE_URL` | yes | Public origin, no trailing slash. Must match the Google redirect URI. |
| `ASSEMBLE_GOOGLE_CLIENT_ID` | yes | From the Google OAuth client. |
| `ASSEMBLE_GOOGLE_CLIENT_SECRET` | yes | From the Google OAuth client. |
| `ASSEMBLE_ADMIN_EMAIL` | yes | The first admin's Google address. It must be verified with Google, and it is bound to that Google account on first sign-in. |
| `ASSEMBLE_DB_PATH` | no | SQLite file. Defaults to `data/assemble.db`; the example compose file sets `/data/assemble.db` on a volume. |
| `ASSEMBLE_LISTEN` | no | Listen address inside the container. Defaults to `:8080`. |
| `ASSEMBLE_DEV_BYPASS` | no | `1` replaces Google with a local test form. Only allowed with an `http://localhost` base URL and a loopback `ASSEMBLE_LISTEN`, so not in a container. See [Development](#development). |

There's no session secret to manage. Sessions are random tokens stored hashed in the database, so restarts and image updates never sign anyone out.

## Hardening

These are suggestions for anyone running `assemble` on a server reachable from the internet. They're ordinary Docker and reverse-proxy practice; none of them is specific to this app. The example compose file already applies the container settings.

- **Keep the container off the internet.** Put the app behind a reverse proxy that terminates TLS, and don't publish its port. If you must publish one, bind it to `127.0.0.1`. Turn on HSTS at the proxy.
- **Restrict the container.** The app writes only to its data volume. Run it with:
  - a read-only root filesystem (`read_only: true`, with a `tmpfs` for `/tmp`)
  - no Linux capabilities (`cap_drop: [ALL]`)
  - `no-new-privileges`
  - modest memory and process limits
- **Rotate logs.** Docker's default log driver keeps container logs forever. Set `max-size` and `max-file`, as the example compose file does.
- **Keep the image current.** Pin a release tag and update it deliberately. Rebuilding or pulling picks up Go and base-image security fixes, so don't leave an image unchanged for many months.
- **Protect the secrets.** `.env` holds the OAuth client secret, so make it readable only by its owner (`chmod 600 .env`). Rotate the secret in Google Cloud if it is ever exposed.
- **Protect the backups.** They contain members' names and email addresses. Store them as you would any personal data.
- **Never set `ASSEMBLE_DEV_BYPASS` on a server.** It replaces Google sign-in with a test form.
- **Watch the logs now and then.** Refused sign-ins and rejected requests are logged, so a sudden run of them is worth a look.

## Development

You need Go 1.25 or later.

```sh
go test ./...
```

To run locally without Google, use the dev sign-in. Any email you type becomes a verified account, so you can try invites and disabling with several "people":

```sh
ASSEMBLE_BASE_URL=http://localhost:8138 ASSEMBLE_LISTEN=127.0.0.1:8138 \
ASSEMBLE_ADMIN_EMAIL=admin@localhost ASSEMBLE_DEV_BYPASS=1 go run .
```

In VS Code, the **assemble: run local (dev sign-in)** task does the same. Sign in as `admin@localhost` to be the admin. Binding `127.0.0.1` keeps the dev server off your network and stops the Windows firewall prompting.

## Docs

- [docs/deployment.md](docs/deployment.md): production deployment, reverse proxy, backups, updates and troubleshooting
- [docs/calendar.md](docs/calendar.md): how days, availability, lock-ins and notices work
- [docs/testplan.md](docs/testplan.md): manual checks after each deploy
- [docs/ui.md](docs/ui.md): the visual style, palette and icons
- [agents.md](agents.md): product design and technical decisions

## Licence

MIT. See [LICENSE](LICENSE).
