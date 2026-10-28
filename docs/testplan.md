# Test plan

Manual checks to walk after each deploy. Tick each **Expect** line. Anything that doesn't match is a defect, not a quirk.

Substitute your own values for `YOUR-DOMAIN`. Don't record real domains, emails or secrets here: this file is committed.

You'll need:
- a phone (it's the target)
- the admin's Google account
- **three other Google accounts**: two members (one becomes a manager in section M) and a stranger

Version under test: **2026.3**: managers, lock-in messages, the MOTD and join notices, Upcoming as home, and help. These sit on 2026.2's calendar and 2026.1's sign-in, invites, members and group settings.

On the local copy (UAT), every page has a muted red header band and a **LOCAL** tag. The server never does.

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
| B3 | Open `https://YOUR-DOMAIN/healthz` | `ok 2026.3 build main:<commit>`: the deployed branch and the first 8 characters of its commit. The page footer matches. The header has **no** red band or **local** tag. |

## C. Admin sign-in

| # | Action | Expect |
|---|---|---|
| C1 | Open `https://YOUR-DOMAIN/` in a private tab on the phone | Padlock. Header shows the group name (default `assemble`). One **Sign in with Google** button. HTTP 401. |
| C2 | Sign in with the `ASSEMBLE_ADMIN_EMAIL` account | Google's account chooser, then back to **Upcoming**, with **Signed in as <name>** and **Admin**, **Help** and **Sign out** links. |
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
| E4 | Under **Language filter**, add a word, e.g. `darn`, and save | Saved, and the list reads back neatly, comma-separated. |
| E5 | As a member, save a note or display name containing that word | Refused: **Blocked by language filter.** A longer word that contains it, like `darned`, is fine. |
| E6 | Text containing the word that was saved before the filter | Shows as `***` in the group list, notices and member list. |

## F. Invites and members

| # | Action | Expect |
|---|---|---|
| F1 | **Admin → Invites → Create invite link** | A link appears with **Copy** (and **Share** on a phone). Created today, 0 joined. |
| F2 | **Share** (or **Copy**) the link to a chat with yourself | The full `https://YOUR-DOMAIN/join/...` link arrives. |
| F3 | Open the link in a private tab | **Join <group name>**. |
| F4 | Sign in with the **member** account | **Upcoming**, with no Admin link. The invite link now shows 1 joined. |
| F5 | As the member, open `/admin/group` | **Admins only**, HTTP 403. |
| F6 | As admin, **Disable** the link, and confirm | Marked disabled; its URL is no longer shown. |
| F7 | Open the disabled link in a private tab | **Invite no longer valid**. |
| F8 | **Admin → Members**: **Disable** the member, and confirm | Marked disabled. |
| F9 | Reload the member's tab | Back to **Sign in**. Signing in gives **Access turned off**. |
| F10 | **Enable** the member; they sign in again | Back on Upcoming. |
| F11 | Look at your own row in **Members** | Marked **(you)**, with no Disable button. |

## G. Persistence and updates

| # | Action | Expect |
|---|---|---|
| G1 | `docker compose down && docker compose up -d` | Settings, invites and members are all kept. Nobody is signed out. |
| G2 | Pull a newer image and `up -d` | Same as G1. The footer shows the new commit. |
| G3 | Take an online backup ([deployment.md](deployment.md#backups)) | A `.db` file that opens in `sqlite3` and has the `users` rows. |

## I. The day view

Signed in as the member, on the phone.

| # | Action | Expect |
|---|---|---|
| I1 | Open the app and tap the top row | Today's date under a **See all upcoming** button, the four status buttons with **?** selected, and the group list with everyone at **?**. |
| I2 | Tap **YES** | YES fills in straight away, with no reload, and gets a green border. Your row in the group list and the phrase above it update, e.g. "Ready to lock in!" once everyone's in, or "1 can't make it". |
| I3 | Reload | YES is still selected. |
| I4 | Swipe left, then tap **NO** | Tomorrow. NO is selected. |
| I5 | Swipe right twice | Back to today, then yesterday. Yesterday is read-only: the buttons are greyed and it says the day has passed. |
| I6 | Go forward to two months from today | The **›** arrow disappears on the last day. |
| I7 | On a day with an answer, type a note and **Save** | **Saved**. The note shows under your name in the group list. |
| I8 | Tap **?** on that day | The note field disappears and the note is gone. |
| I9 | As the admin, open the same day | The member's status and note are there. |

## J. Lock-in and notices

Admin and member each on their own device or browser.

| # | Action | Expect |
|---|---|---|
| J1 | Both set **YES** on a day. The admin taps **Lock in this day**, leaving the message as it is | No confirm. The **Locked in** banner appears for both, and the line under **The group** reads "Locked in for 19:00 to 22:00" (your timeslot). |
| J2 | Both see a notice at the top: **Locked in: <day>** | The admin's says "by you". The member's names the admin. |
| J3 | The member taps **×** on the notice | It goes at once, and stays gone after a reload. The admin still has theirs. |
| J4 | The member taps **NO** on the locked-in day and **cancels** the confirm | Nothing changes: still YES, still locked in. |
| J5 | Again, and **confirms** | The banner disappears. Both get **Cancelled: <day>**, naming the member. |
| J6 | On another day, the member is **?**. The admin taps **Lock in this day** | The confirm names the member. Accepting locks it in, and the banner says not everyone said they could. |
| J7 | The member changes **?** to **NO** there | No confirm. It stays locked in: they weren't available before either. |
| J8 | The admin taps **Cancel the locked-in session** and confirms | Unlocked. Everyone gets a cancellation notice. |
| J9 | The day after a notice's day | The notice has gone without being dismissed. |

## K. Upcoming, profile and public pages

| # | Action | Expect |
|---|---|---|
| K1 | **See all upcoming** from a day, or the logo in the header | Two months of days under month headings, each with "Others" and "You" over the columns. Each row has a bar of the other members' answers and your own status. Locked-in days have a green padlock and a green outline; days everyone can make, which could be locked in, have a wisteria outline. |
| K2 | Tap a row | That day opens. |
| K3 | Tap **change display name** next to **Signed in as**. Set a display name and save | **Saved**. The group list and other members now show the new name. |
| K4 | Sign out and back in | The display name is kept, not replaced by your Google name. |
| K5 | Clear the display name | Back to your Google name. |
| K6 | Signed out, open `/privacy` and `/terms` | Both load without sign-in. The sign-in and join pages link to them. |
| K7 | The footer on any page | The version on the left, and the GitHub mark on the right linking to the repo. |

## M. 2026.3: managers, messages and help

Admin, the member from F and a second member, each on their own device or browser.

| # | Action | Expect |
|---|---|---|
| M1 | Invite the second member and have them join | The admin gets **<name> joined the group**, with no link. It stays until dismissed, whatever the date. The first member gets nothing. |
| M2 | **Admin → Members**: **Make manager** on the first member | Tagged **manager**, with **Remove manager**. The admin's own row has neither button. |
| M3 | As the manager, open a day | **Lock in this day** with a message field pre-filled "Locked in for 19:00 to 22:00". No Admin link; `/admin/group` is **Admins only**. |
| M4 | The manager changes the message to e.g. "From 8pm at the studio" and locks in | Everyone sees that message as the group line. The notices say the day is locked in, by the manager. |
| M5 | The manager rewords it under **Lock-in message** and taps **Save** | Everyone sees the new wording. No new notice for anyone. |
| M6 | The manager cancels it | Unlocked, and everyone is notified. |
| M7 | The admin taps **Remove manager**, then the member reloads a day | No lock-in controls. |
| M8 | Lock in a day with **?** answers, editing the message, and accept the override | The message you typed is the one saved, with JS on or off. |
| M9 | **Admin → Group**: post a message of the day | **Message of the day posted.** It's at the top of everyone's pages, the admin's saying "by you". |
| M10 | A member dismisses it, then the admin posts the same text again | It stays dismissed. |
| M11 | The admin posts different text | It's back for everyone, with the new text. The old one has gone. |
| M12 | The admin empties the field and posts | **Message of the day removed.** It's gone for everyone. |
| M13 | A message over 100 characters, or one with a filtered word | Refused with a message, for both the MOTD and a lock-in message. |
| M14 | **Help** in the menu | The four answers with their icons, the good habits, locked-in days and notices. "assemble" in the first line opens the GitHub repo in a new tab. Signed out, `/help` asks you to sign in. |
| M15 | Open the old address `/upcoming` | Lands on home. |

## L. Phone and look

| # | Action | Expect |
|---|---|---|
| L1 | Every page at 360px wide (small Android) and 412px (Pixel) | No sideways scrolling. Buttons at least a thumb wide. All four admin tabs visible. |
| L2 | **Admin → Style** | The palette, the four status icons (YES ✓, OK ~, NO ✕, ?) and the type in Manrope. |
| L3 | Turn on the phone's large text setting | Text scales, layout holds. |
