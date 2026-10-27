# About assemble

`assemble` helps groups to easily determine common availability for group activities. 

The core use case is `band practice`, where multiple people need to coordinate their schedules to all be available at the same time to get together. Obviously if the group reliably gets together at the same time every week/fortnight/month this tool isn't needed - but as life gets more complex, so do schedules. The app can also apply to DND nights, book clubs, etc.

# Core Use Cases

* As a group member, I can logon to `assemble` using Google Auth
* Primary: As a group member, I can easily specify my availability each day in the default timeslot with status [YES] (preferred), [OK] (possible but not preferred) or [NO] (not available). I can also revert to the default [?] (unspecified) from another status
* As a group member, I can see group member status per user per day, including [?] 
* as a group member, I can see locked-in bookings
* as a group member, I can see and dismiss in-app notifications
* As an instance admin, I can set the name of the group, the default timeslot and timezone
* as an instance admin, I can enable/disable access for other users
* as an instance admin, I can generate and disable group invite links
* as an instance admin, I can mark/unmark a day as `locked-in`
* as a group member, I can add a public note to my answer for a day
* as a group member, I can choose the display name the group sees
* as an instance admin, I can keep a language filter for free text

# FRs

* generate in-app notifications: session locked-in, session cancelled
* Initial user registration from an admin-generated invite link
* Mobile-first design
* Google Auth login with caching to enable seamless use
* initially assume all group members are required, but admin can override lock-in for undersubscribed sessions
* Date horizon: rolling 2 months in advance, past days read-only, read-only set once the full day is in the past in the group timezone
* Invite links are multi-use. Intent is that admin posts the link to the group chat, all users register, then the admin disables the invite link.
* UI should be optimised for minimal taps on the core primary use case - view existing availability for today > opt: tap availability icon to set/change > opt: visually verify > opt: swipe to next day. Availability icons are a set of 4 visible ( only single-select possible) rather than nested underneath a single icon.
* Admin can lock-in when every enabled member is [YES] or [OK]. Anything else needs the admin to confirm an override to lock-in. [?] counts as not available. 
* A user changing their status from either [YES] or [OK] to either [?] or [NO] on a locked-in date will cancel that lock-in and trigger the notifications. 
* The user should be prompted to confirm ("this will cancel the locked-in session, are you sure?") if their tapped action will result in session cancellation.
* Disabled users are excluded from rule tests (e.g. checking if a session can be locked-in or should be cancelled)
* notifications are displayed to all users, including the one who triggered it, and can be dismissed with a tap (e.g. on an X icon) preventing them appearing again
* notifications auto-expire once the day they relate to is in the past

# NFRs

* Developed as a generic solution in a public repo - no secrets to be committed to the repo
* Assume one timeslot per day in the specified TZ (e.g. 1900-2200 Australia/Brisbane)
* Deployed on Docker and updated from GitHub
* Single-group per Docker instance. No need for multiple group, user segmentation etc
* Likely low-volume use per instance. Small groups providing per-day availability a few weeks out at most. 
* Data should survive a docker/host restart (requires persistence)
* Low maintenance overhead - avoid highly changeable UI libraries etc. Should be able to run for 6mth+ without needing sysadmin updates
* Initial admin configured by env var (for Google Auth)

# Future

* Email/Telegram reminders (upcoming booking, reminder to add availability, cancellation etc)
* generate in-app notifications to admin: lock-in a session that has full group availability
* add confirmed bookings to GCal from the webapp
* Admin mark users as `required` or `optional`
* Admin grants admin-lite role to other users (e.g. can lock-in or cancel, no user management)
* Events - e.g. a gig date that the group is practicing for. This is a fixed-date event triggering notifications until all availabilities are specified

# Stack

* **Go** - single static binary serving the UI and API in one process.
* **Go stdlib plus one module: `modernc.org/sqlite`.** It is pure Go, so `CGO_ENABLED=0` and a `distroless/static` base still work. The bar for adding any other module is "nothing that churns". Google sign-in is two HTTP calls done by hand, not `x/oauth2` or `go-oidc`.
* **stdlib `html/template`**, server-rendered. No templ, because it needs codegen and a CLI in the build path.
* **Small vanilla JS islands, no framework:** the day view (tap to set status, notes, swipe between days), dismissing notices, and copy/share/confirm on the admin pages. They post JSON or enhance plain forms, and every page still works without JS. Reconsider htmx only if pages start needing to swap fragments of markup; then vendor and pin it.
* **SQLite** on a Docker volume, in WAL mode, never baked into the image. Back up by copying the file (or with `VACUUM INTO`).
* **Days are calendar dates in the group timezone** (`YYYY-MM-DD`), not UTC instants. A day's availability never moves when DST changes or when the admin changes the timezone. Store the timezone as an IANA name (e.g. `Australia/Sydney`), not an abbreviation like AEST. Import `time/tzdata` so zones resolve whatever the base image is.
* **One container.** GitHub Actions builds the image and publishes it to GHCR. `VERSION` (CalVer `YYYY.n`) drives the release tag.
* **All config comes from env vars.** `.env.example` and `docker-compose.example.yml` are committed. The real files are gitignored.

Chosen over SvelteKit (npm churn works against the 6-month unattended goal) and PocketBase (pre-1.0, and it ships breaking changes between minor versions).

# Auth

Google sign-in is used for identity only (OpenID Connect, scopes `openid email profile`). After that, `assemble` issues its own session. No refresh tokens, no offline access, and no further contact with Google until the session expires.

* **Identity is the Google `sub`, not the email.** Emails can be reassigned to someone else, but a `sub` can't. Email and name are stored for display only.
* **Bootstrap admin is set by `ASSEMBLE_ADMIN_EMAIL`.** On sign-in, a Google account with a matching, verified email (`email_verified=true`) becomes admin and is bound to its `sub`. This is the only place the email is used to identify someone, because a `sub` can't be known before the first sign-in.
* **Registration is by invite only.** `/join/<token>` stores the token in a short-lived cookie and starts the Google flow. If the invite is still active, the callback creates the member. Anyone else whose `sub` isn't known is refused. Tokens are 32 random bytes. Disabling a link stops new registrations and leaves existing members alone.
* **A disabled user stays disabled**, even if they come back through an active invite link.
* **Sessions are stored server-side.** The browser gets a random token in a cookie set `HttpOnly`, `Secure` and `SameSite=Lax`. SQLite stores only a hash of the token. Expiry is a rolling 6 months, well inside Chrome's 400-day cookie cap. Each request's session lookup also checks that the user is still enabled, so disabling someone takes effect on their next request. With no signed cookies there's no session secret to manage.
* **The cookie's `Secure` flag comes from `ASSEMBLE_BASE_URL`, not from the request**, because TLS terminates at the reverse proxy.
* **Google Cloud setup, per deployer:** create an OAuth client of type "Web application" with redirect URI `<ASSEMBLE_BASE_URL>/auth/callback`.
  * **The consent screen must be published to "In production".** While it's in Testing, only manually listed test users can sign in, which breaks invite links. The basic scopes are non-sensitive, so publishing doesn't need Google verification.
  * Google allows `http` redirect URIs only for `localhost`/`127.0.0.1`. A real deployment needs a domain and TLS, typically via a reverse proxy.
* **Env config:** `ASSEMBLE_BASE_URL`, `ASSEMBLE_GOOGLE_CLIENT_ID`, `ASSEMBLE_GOOGLE_CLIENT_SECRET`, `ASSEMBLE_ADMIN_EMAIL`. Group name, timeslot and timezone are admin settings kept in the database, not env vars.