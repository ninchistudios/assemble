# UI

The visual style for every page. To see it on a real device, sign in as admin and open `/style`.

## Principles

- **Flat and vector.** No gradients, shadows or textures. Icons are inline SVG, so they stay sharp at any size and take their colours from the theme.
- **Dark first.** Dark is the default theme. Light and other themes come later, built on the same tokens.
- **Phone first.** Designed at 360px wide and checked at 360 and 412. Touch targets are at least 48px. There is never horizontal scrolling.
- **Few taps.** The main task, setting your availability for a day, takes one tap once the day is open. From home (Upcoming, since 2026.3) that's two: the day, then the status.
- **Nothing third-party.** The font, icons and styles are all embedded in the binary. The Content Security Policy blocks inline styles and scripts, so styling goes in `assemble.css` and never in `style=` attributes.

## Tokens and themes

Every colour is a CSS custom property set in a token block at the top of `internal/web/static/assemble.css`. The rest of the stylesheet uses only the tokens and never names a colour directly.

The theme is chosen by `data-theme` on `<html>`, which is currently always `dark`. To add a theme, add a `:root[data-theme="name"] { ... }` block that gives every token a value. No markup or component CSS has to change. Choosing a theme per user or per group is a future feature.

## Palette (dark)

The palette is small and distinctive, and it avoids primary colours. Status colours keep their usual meanings: greenish is good, amber is possible, reddish is bad, grey is unknown.

| Token | Value | Name | Use | Contrast on `bg` |
|---|---|---|---|---|
| `--bg` | `#15171c` | ink | page background | |
| `--surface` | `#1e2128` | | rows, fields, quiet buttons | |
| `--surface-2` | `#272b34` | | selected tab, tags | |
| `--line` | `#323743` | | field borders | |
| `--text` | `#efeae2` | parchment | body text | 15.0 |
| `--muted` | `#9ba1ad` | | secondary text | 6.9 |
| `--accent` | `#b4a2f0` | wisteria | links, primary buttons, focus | 8.0 |
| `--yes` | `#74c69d` | sage | [YES] | 8.8 |
| `--ok` | `#e6b450` | saffron | [OK] | 9.4 |
| `--no` | `#ec7a64` | coral | [NO], destructive actions | 6.4 |
| `--unknown` | `#6b7280` | slate | [?] | 3.7 |
| `--env-local` | `#4a2420` | | header band on a local copy (#20) | |

- All text meets WCAG AA (4.5:1).
- `--unknown` is used only as an icon fill, where the requirement for non-text graphics is 3:1. It is never used for text.
- Glyphs and labels on any coloured fill use `--bg` (`--on-status`, `--on-accent`, `--on-danger`). That gives at least 6.4:1 on every fill except slate.

**Status never relies on colour alone.** Every status has a glyph, and wherever there's room, a text label as well. This matters because red–green colour blindness affects about 1 in 12 men.

## Icons

- **Shape:** every icon sits in a rounded-corner square, with a corner radius of about 23% of its side (`rx="11"` on a 48 box). The app mark uses the same square for its tile and for its four inner squares.
- **Status icons:** a square filled with the status colour, with a stroked glyph in `--on-status`, stroke width 4.5 on a 48 viewBox with round caps and joins.

  | Status | Glyph | Label |
  |---|---|---|
  | yes | check | YES |
  | ok | wave | OK |
  | no | cross | NO |
  | unknown | question mark | ? |

- **Picker:** in a choice of status, the chosen option is filled and its button gets a solid border in the same status colour. The other options are outlined, with the glyph drawn in the status colour. The shape stays the same and only the fill changes.
- **Sizes:** 48px for anything you tap, 28px (`.small`) for display only.
- **Other icons, same frame:** a padlock (`lock-icon`) for locked-in days, filled with `--yes`; a cross (`x-icon`) to dismiss a notice. The footer's GitHub mark (`github-icon`, from Octicons) is the one exception to the square frame, because it's GitHub's own mark.
- **Where they live:** `templates/icons.html` (`status`, `status-off`, `mark`, `lock-icon`, `x-icon`, `github-icon`). The favicon `static/icon.svg` repeats the mark with literal colours, because a favicon can't read CSS variables.

## Typography

- **Font:** Manrope, a variable font with weights 200–800 under the SIL Open Font License. It is self-hosted in `static/fonts/` (Latin and Latin Extended subsets, about 40KB), with its licence in `OFL.txt`. Characters outside those subsets fall back to the system sans.
- The base size is 17px at weight 500. That weight is what makes body text feel substantial without shouting.

| Role | Size | Weight |
|---|---|---|
| Page heading (`h2`) | 1.6rem | 800, slightly tightened letter spacing |
| Group name (header) | 1.35rem | 800 |
| Section (`h3`) | 1.1rem | 700 |
| Labels, buttons | 1rem | 700–750 |
| Body | 1rem | 500 |
| Meta, footer | 0.75–0.85rem | 500–600, `--muted` |
| Tags | 0.75rem | 750, uppercase, wider letter spacing |

**Casing:** sentence case for headings, labels and buttons. Status labels are uppercase (YES, OK, NO), as the spec writes them.

## Components

All of these are shown on `/style`.

- **Buttons:** primary buttons (`.btn`) are full width and filled with `--accent`. `.btn-top` is a slimmer (40px) primary button at the top of a page, for **See all upcoming** on a day (2026.3), so the status picker under it stays the focus. `.btn-secondary` is full width too, on `--surface-2`, for a second main action. `.btn-quiet` and `.btn-danger` size to their label, but are never narrower than half the page width. `.btn-small` is for actions inside a row. All are at least 40px tall, and 48px except in rows.
- **Fields:** a `--surface` fill with a 2px `--line` border, which turns `--accent` when focused.
- **Rows:** `.rows > li`, a `--surface` card with an optional leading icon, a growing text block with a `.meta` line underneath, and a trailing `.tag`.
- **Notices:** `.notice` with a coloured left edge: accent for information, `.ok` for success, `.error` for problems.
- **Tabs:** `.tabs` for switching between admin sections. The current tab is marked with `aria-current="page"`.

The calendar adds these (2026.2):

- **Status picker:** `.picker`, four equal buttons with the status icon at 56px. Each button holds both the filled and outlined icon. `aria-pressed` decides which shows, so the script changes one attribute and the page never redraws.
- **Group list:** `.rows.group`, one row per member with a 32px status icon, their name, their note as `.meta` and the status word. The viewer's own row has an accent edge.
- **Lock banner:** `.lock-banner`, a sage (`--yes`) outline with the padlock. Since 2026.3, locked in is sage, the outcome everyone's answers were for, and the accent marks a day that could be locked in (#35).
- **Notices:** `.notices`, at the top of every signed-in page. Locked-in notices have a sage edge, the MOTD an accent one, cancellations a coral one, and a member joining a muted one. A notice about a day links to it; the others are plain text (`.notice-text` either way) and wrap as needed. Each has a 44px dismiss target.
- **Upcoming rows:** `.up-row`, the date, a `.bar` of one equal segment per other member (yes, ok, no, ? in that order, so no inline widths) and the viewer's own status. Each month heading carries the column headings "Others" and "You", centred over their columns. A sage outline means locked in; a wisteria outline means everyone can make it, so it could be locked in.
- **Lock-in message (2026.3):** `.lock-form`, a labelled field above **Lock in this day**, or with a small **Save** beside it (`.msg-row`) to reword it on a locked-in day. On a locked-in day the group line (`.summary.locked`) is the message, in sage at body size.
- **Menu:** the `menu` template, at the foot of home, a day and Help: Admin (admin only), Help and Sign out, then **Signed in as**.
- **Help:** `help.html` reuses the group rows for the four statuses, each with its icon and meaning. "assemble" in its first line links to the repo in a new tab, like the footer's GitHub mark.
- **MOTD form:** at the top of **Admin → Group**, set apart from the settings by a rule (`.motd-form`).
- **Local band:** `body.env-local` gives the header a full-width `--env-local` band and a coral **LOCAL** tag when the base URL is localhost.
