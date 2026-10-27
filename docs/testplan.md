# Test plan

Manual checks to walk after each deploy. Tick each **Expect** line. Anything that doesn't match is a defect, not a quirk.

Substitute your own values for `YOUR-DOMAIN`. Don't record real domains, emails or secrets here: this file is committed.

You'll need:
- a phone (it's the target)
- the admin's Google account
- **two other Google accounts**, for a member and a stranger

Version under test: **2026.1**: sign-in, invites, members, group settings.

---

## A. Before deploying

| # | Action | Expect |
|---|---|---|
| A1 | `go test ./...` | All packages pass. |
| A2 | Start with `ASSEMBLE_ADMIN_EMAIL` unset | Exits at once: `missing required environment: ASSEMBLE_ADMIN_EMAIL`. |
| A3 | Start with `ASSEMBLE_DEV_BYPASS=1` and an `https://` base URL | Refuses to start. This is the guard that keeps the bypass out of production. |
| A4 | `docker compose up -d`, then `docker compose ps` | The container reaches **healthy** within about a minute. |

## B. Google setup, as a new user

Follow the [README](../README.md#1-create-a-google-oauth-client) from scratch, with a fresh Google Cloud project if you can. The point is to check the instructions as much as the app.

| # | Action | Expect |
|---|---|---|
| B1 | Follow README step 1 exactly as written | Every screen and label named in it exists. Note anything that differs. |
| B2 | Check the consent screen's publishing status | **In production**. |
| B3 | Open `https://YOUR-DOMAIN/healthz` | `ok 2026.1 <commit>`. |

## C. Admin sign-in

| # | Action | Expect |
|---|---|---|
| C1 | Open `https://YOUR-DOMAIN/` in a private tab on the phone | Padlock. Header shows the group name (default `assemble`). One **Sign in with Google** button. HTTP 401. |
| C2 | Sign in with the `ASSEMBLE_ADMIN_EMAIL` account | Google's account chooser, then back to **Hi, <name>** with an **Admin** button. |
| C3 | `docker compose logs assemble \| grep auth:` | `bootstrapped admin user=1` and `signed in user=1`. |
| C4 | Close and reopen the browser, then open the site | Still signed in, no prompt. |
| C5 | Add the site to the home screen and open it from there | Lands signed in, in one tap. |

## D. Strangers

| # | Action | Expect |
|---|---|---|
| D1 | In a private tab, sign in with the **stranger** account | **Not a member**, HTTP 403, showing the email used. No group content. |
| D2 | Tap **Try a different account** | Google's account chooser. |

## E. Group settings

| # | Action | Expect |
|---|---|---|
| E1 | **Admin → Group**: set a name, 19:00–22:00 and your timezone, e.g. `Australia/Brisbane`, then **Save** | **Saved.** The header shows the name and `19:00–22:00 · Australia/Brisbane`. |
| E2 | Enter the timezone `AEST` and save | Refused with a message; nothing changes. |
| E3 | `docker compose restart assemble`, then reload | Settings kept; still signed in. |

## F. Invites and members

| # | Action | Expect |
|---|---|---|
| F1 | **Admin → Invites → Create invite link** | A link appears with **Copy** (and **Share** on a phone). Created today, 0 joined. |
| F2 | **Share** (or **Copy**) the link to a chat with yourself | The full `https://YOUR-DOMAIN/join/...` link arrives. |
| F3 | Open the link in a private tab | **Join <group name>**. |
| F4 | Sign in with the **member** account | **Hi, <name>** with no Admin button. The link now shows 1 joined. |
| F5 | As the member, open `/admin/group` | **Admins only**, HTTP 403. |
| F6 | As admin, **Disable** the link, and confirm | Marked disabled; its URL is no longer shown. |
| F7 | Open the disabled link in a private tab | **Invite no longer valid**. |
| F8 | **Admin → Members**: **Disable** the member, and confirm | Marked disabled. |
| F9 | Reload the member's tab | Back to **Sign in**. Signing in gives **Access turned off**. |
| F10 | **Enable** the member; they sign in again | **Hi, <name>**. |
| F11 | Look at your own row in **Members** | Marked **(you)**, with no Disable button. |

## G. Persistence and updates

| # | Action | Expect |
|---|---|---|
| G1 | `docker compose down && docker compose up -d` | Settings, invites and members are all kept. Nobody is signed out. |
| G2 | Pull a newer image and `up -d` | Same as G1. The footer shows the new commit. |
| G3 | Take an online backup ([deployment.md](deployment.md#backups)) | A `.db` file that opens in `sqlite3` and has the `users` rows. |

## H. Phone and look

| # | Action | Expect |
|---|---|---|
| H1 | Every page at 360px wide (small Android) and 412px (Pixel) | No sideways scrolling. Buttons at least a thumb wide. All four admin tabs visible. |
| H2 | **Admin → Style** | The palette, the four status icons (YES ✓, OK ~, NO ✕, ?) and the type in Manrope. |
| H3 | Turn on the phone's large text setting | Text scales, layout holds. |
