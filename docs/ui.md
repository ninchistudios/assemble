# UI

The visual style for every page. To see it on a real device, sign in as admin and open `/style`.

## Principles

- **Flat and vector.** No gradients, shadows or textures. Icons are inline SVG, so they stay sharp at any size and take their colours from the theme.
- **Dark first.** Dark is the default theme. Light and other themes come later, built on the same tokens.
- **Phone first.** Designed at 360px wide and checked at 360 and 412. Touch targets are at least 48px. There is never horizontal scrolling.
- **Few taps.** The main task, setting your availability for a day (2026.2), should take one tap.
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

- **Picker:** in a choice of status, the chosen option is filled. The other options are outlined, with the glyph drawn in the status colour. The shape stays the same and only the fill changes.
- **Sizes:** 48px for anything you tap, 28px (`.small`) for display only.
- **Where they live:** `templates/icons.html` (`status`, `status-off`, `mark`). The favicon `static/icon.svg` repeats the mark with literal colours, because a favicon can't read CSS variables.

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

- **Buttons:** primary buttons (`.btn`) are full width and filled with `--accent`. `.btn-quiet` and `.btn-danger` size to their label, but are never narrower than half the page width. They are all at least 48px tall.
- **Fields:** a `--surface` fill with a 2px `--line` border, which turns `--accent` when focused.
- **Rows:** `.rows > li`, a `--surface` card with an optional leading icon, a growing text block with a `.meta` line underneath, and a trailing `.tag`.
- **Notices:** `.notice` with a coloured left edge: accent for information, `.ok` for success, `.error` for problems.
- **Tabs:** `.tabs` for switching between admin sections. The current tab is marked with `aria-current="page"`.
