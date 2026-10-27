# Calendar design

How availability, lock-ins and notifications work in 2026.2. The product rules come from [agents.md](../agents.md); this is how they're built. The code is in `internal/cal` (rules), `internal/store` (storage) and `internal/web` (pages).

## Days

A **day** is a calendar date in the group's timezone, stored as text `YYYY-MM-DD`. It is never stored as an instant (a moment in time).

- **Today** is the current date on the group's clock, not the server's. At 23:30 UTC on 1 November, Brisbane is already on 2 November.
- **Changing the timezone never moves a stored day.** 14 November stays 14 November.
- **Date arithmetic** (next day, two months ahead) works on dates alone, so daylight-saving changes can't skip or repeat a day.
- **Dates sort as text,** so range queries are plain string comparisons.

There is one timeslot per day (group settings). The calendar doesn't use its times for anything except display.

### The window

| Days | Rule |
|---|---|
| Before today | **Past.** Visible but read-only. A day becomes past once it has fully ended in the group timezone. |
| Today to today + 2 months | **Editable.** Members set their status; the admin locks in and cancels. |
| After that | **Beyond the horizon.** Can't be opened. |

## Statuses

| Status | Stored | Counts as available |
|---|---|---|
| [YES], preferred | `yes` | yes |
| [OK], possible | `ok` | yes |
| [NO], not available | `no` | no |
| [?], not answered | no row | no |

- **Setting [?]** deletes the member's row for that day, note included.
- **Disabled members** are left out of everything: they aren't listed or counted, and they never block or trigger a lock-in rule.
- **Each member can add a public note per day** (#15), e.g. "can't make it until 8pm". The note belongs to the answer, so it goes when the status goes back to [?].

## Lock-in

A locked-in day is a booking: the group meets that day.

- **Locking in:** only the admin can lock in, and only an editable day.
  - **Normal lock-in:** every enabled member is [YES] or [OK].
  - **Otherwise:** the admin has to confirm an override. [?] counts as not available.
  - **What's stored:** a `lockins` row records who locked it, when, and whether it was an override.
- **Cancelling:** the admin can cancel a locked-in day, which deletes the row.
- **A member pulling out cancels:** a member changing from [YES]/[OK] to [NO]/[?] on a locked-in day cancels it, after a confirm ("This will cancel the locked-in session. Are you sure?").
  - **Nothing else cancels.** On a day locked by override, a member moving from [NO] to [?] leaves the lock alone.
  - **On the server:** the change is refused with "needs confirmation" unless the request carries a confirm flag. The page shows the confirm, then resends with the flag. Without JS, the same exchange is a small confirm page with a form.

## Notifications

- **Created:** one record per lock-in and per cancellation, whether the admin cancels or a member's change does. Each records who triggered it.
- **Shown to:** every enabled member, including whoever triggered it, at the top of their pages.
- **Dismissed:** per person with one tap. A `dismissals` row makes it stay dismissed for that member only.
- **Expire:** once their day is past. Expired notices are simply not shown; they don't need deleting.

## Screens

### Day view (home)

`/` shows today, and `/day/YYYY-MM-DD` shows any other day in the window. Top to bottom:

1. **The date** ("Saturday 14 November") and a locked-in marker if the day is booked.
2. **My status:** four buttons, [YES] [OK] [NO] [?]. The current one is filled and the others outlined. One tap sets it.
3. **My note** (#15).
4. **The group:** every enabled member with their status and note, me first, under a phrase saying where the day stands. The first rule that fits wins:

   | When | Phrase |
   |---|---|
   | Anyone is [NO] | "*n* can't make it" |
   | Everyone, me included, is [YES]/[OK] | "Ready to lock in!" |
   | Everyone else is in, and I haven't said | "All others are in, great option!" |
   | Nobody is in yet | "No answers yet" |
   | Otherwise | "*n* are in, it might work..." |

   The server words the phrase and sends it back with each status change, so the page script never repeats these rules.
5. **Admin only:** Lock in / Cancel.
6. **Previous / next day.**

**Speed.** Tapping a status uses a small script that sends JSON and updates the page in place, with no reload. Swiping left or right moves a day. Without JS, the buttons are ordinary form submits and the page reloads. Both paths use the same handler.

**Out-of-window days.** Past days show the same layout, read-only. Days beyond the horizon return 404.

### Upcoming days (#28)

One compact row per day in the window: the date, a bar of the **other** members' answers (one equal segment each, so no inline widths), my own status in its own column, and a padlock if it's locked in. Each month heading also carries the column headings "Others" and "You". A day stands out when every enabled member, me included, is available. Tapping a row opens that day.

## Storage

All in migration 3 (`internal/store/schema.go`):

| Table | Key | Holds |
|---|---|---|
| `availability` | (user, day) | status, note, updated time |
| `lockins` | day | who, when, override |
| `notifications` | id | kind (`locked`/`cancelled`), day, who triggered it, when |
| `dismissals` | (notification, user) | which member dismissed which notice |
| `users.display_name` | (column) | the name a member chose to be shown by (#14); empty means their Google name. Google sign-ins never overwrite it. |

Deleting a user deletes their answers and dismissals. Users are never deleted in practice; they are disabled.
