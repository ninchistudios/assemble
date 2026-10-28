# Calendar design

How availability, lock-ins and notifications work, as of 2026.3. The product rules come from [agents.md](../agents.md); this is how they're built. The code is in `internal/cal` (rules), `internal/store` (storage) and `internal/web` (pages).

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
| Today to today + 2 months | **Editable.** Members set their status; the admin and managers lock in and cancel. |
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

- **Who:** the admin and managers (#39). A manager is a member with the `manager` flag, which only the admin grants and revokes on **Admin → Members**. The admin always counts as a manager, whatever the flag says. Managers can't open the admin pages.
- **Locking in:** only an editable day.
  - **Normal lock-in:** every enabled member is [YES] or [OK].
  - **Otherwise:** the admin or manager has to confirm an override. [?] counts as not available.
  - **What's stored:** a `lockins` row records who locked it, when, whether it was an override, and its message.
- **The message (#41):** the lock-in form has a message field, pre-filled "Locked in for 19:00 to 22:00" from the timeslot. What's saved is whatever is left in the field, so a one-off change of time or place goes in the message rather than the settings. Up to 100 characters, under the language filter.
  - On a locked-in day the message replaces the group's status line, whatever the answers. Empty shows "Locked in".
  - The admin or a manager can reword it later. That notifies nobody.
  - A form without the field gets the default, and the override's confirm page carries the message through.
- **Cancelling:** the admin or a manager can cancel a locked-in day, which deletes the row.
- **A member pulling out cancels:** a member changing from [YES]/[OK] to [NO]/[?] on a locked-in day cancels it, after a confirm ("This will cancel the locked-in session. Are you sure?").
  - **Nothing else cancels.** On a day locked by override, a member moving from [NO] to [?] leaves the lock alone.
  - **On the server:** the change is refused with "needs confirmation" unless the request carries a confirm flag. The page shows the confirm, then resends with the flag. Without JS, the same exchange is a small confirm page with a form.

## Notifications

| Kind | Created when | Shown to | Links to | Expires |
|---|---|---|---|---|
| `locked` | a day is locked in | everyone | the day | once the day is past |
| `cancelled` | a lock-in is cancelled, by the admin, a manager or a member pulling out | everyone | the day | once the day is past |
| `joined` (#38) | someone joins through an invite | the admin and managers | nothing | never |
| `motd` (#40) | the admin posts a message of the day | everyone | nothing | never |

- **Recorded:** in the same transaction as the change it reports, with who triggered it. Shown "by you" to them and "by *name*" to everyone else. A join names the new member instead.
- **Shown:** at the top of every signed-in page, including to whoever triggered it.
- **Dismissed:** per person with one tap. A `dismissals` row makes it stay dismissed for that member only.
- **Expired:** day notices go once their day is past. They are simply not shown, so they don't need deleting. Notices not about a day stay until dismissed.
- **The MOTD:** one at a time, up to 100 characters, under the language filter. Posting new text deletes the old notice and its dismissals, so it shows again to everyone. Posting the same text changes nothing. Posting an empty field removes it. It's set at the top of **Admin → Group**.

## Screens

### Day view

`/day/YYYY-MM-DD` shows a day in the window. Top to bottom:

1. **See all upcoming:** back to home (#35).
2. **The date** ("Saturday 14 November") and a locked-in marker if the day is booked.
3. **My status:** four buttons, [YES] [OK] [NO] [?]. The current one is filled and the others outlined. One tap sets it.
4. **My note** (#15).
5. **The group:** every enabled member with their status and note, me first, under a phrase saying where the day stands. The first rule that fits wins:

   | When | Phrase |
   |---|---|
   | The day is locked in | Its message (#41), in sage |
   | Anyone is [NO] | "*n* can't make it" |
   | Everyone, me included, is [YES]/[OK] | "Ready to lock in!" |
   | Everyone else is in, and I haven't said | "All others are in, great option!" |
   | Nobody is in yet | "No answers yet" |
   | Otherwise | "*n* are in, it might work..." |

   The server words the phrase and sends it back with each status change, so the page script never repeats these rules.
6. **Admin and managers only:** lock in with a message, or reword the message and cancel.
7. **The menu:** Admin (admin only), Help, Sign out, and **Signed in as**.

Previous and next day are the arrows either side of the date.

**Speed.** Tapping a status uses a small script that sends JSON and updates the page in place, with no reload. Swiping left or right moves a day. Without JS, the buttons are ordinary form submits and the page reloads. Both paths use the same handler.

**Out-of-window days.** Past days show the same layout, read-only. Days beyond the horizon return 404.

### Upcoming days: home (#28, #35)

`/` is home: the whole window at a glance, with today as the top row. Its 2026.2 address, `/upcoming`, redirects there. One compact row per day in the window: the date, a bar of the **other** members' answers (one equal segment each, so no inline widths), my own status in its own column, and a padlock if it's locked in. Each month heading also carries the column headings "Others" and "You". A day stands out when every enabled member, me included, is available. Tapping a row opens that day. The menu is at the bottom, as on a day.

### Help (#36)

`/help`, for signed-in members only, linked from the menu. It explains the four answers ([?] is "not sure or not answered", and counts as not available for locking in), the good habits (answer every day, use NO sparingly, notes are public, answers can change until the day is over), locked-in days and pulling out, notices, and getting around.

## Storage

Migration 3 (2026.2) and migration 5 (2026.3), in `internal/store/schema.go`:

| Table | Key | Holds |
|---|---|---|
| `availability` | (user, day) | status, note, updated time |
| `lockins` | day | who, when, override, message |
| `notifications` | id | kind, day (NULL if not about a day), audience (`all`/`managers`), body (the MOTD), who triggered it, when |
| `dismissals` | (notification, user) | which member dismissed which notice |
| `users.display_name` | (column) | the name a member chose to be shown by (#14); empty means their Google name. Google sign-ins never overwrite it. |
| `users.manager` | (column) | 1 for a manager (#39). Ignored on the admin. |

Migration 5 rebuilds `notifications`, because SQLite can't relax its old CHECK on `kind` or its NOT NULL on `day` in place. `dismissals` refers to it and would be emptied by the cascade when the old table is dropped, so its rows are set aside and restored in the same transaction.

Deleting a user deletes their answers and dismissals. Users are never deleted in practice; they are disabled.
