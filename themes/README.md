# Smugbox themes

A theme is a skin for the photo-album app: a `theme.json`, a `theme.css` and
assets (fonts, images). It ships no JavaScript and no HTML; the markup is the
same for every theme. Pick one with `SITE_THEME=<id>` and restart.

This folder holds the themes in the repository. **Noir** is the built-in theme
(the only one in the Docker image); **Glass**, **Nyx**, **Nyx2** and
**Nyx2noir** are user themes.

## Installing a theme

Copy the theme, as a zip or an unzipped folder, into `<DATA_DIR>/themes`
(`/data/themes` in the container), set `SITE_THEME` to its id, restart.

- The id is the zip or folder name without `.zip`, and must match
  `^[a-z0-9][a-z0-9-]*$`.
- A theme in `<DATA_DIR>/themes` wins over a built-in one with the same id.
- Both `<id>.zip` and `<id>/` in the same folder, or an invalid or missing
  selected theme, stop the server from starting; the log says why.
- Built-in themes live in `BUILTIN_THEMES_DIR` (`/srv/themes` in the image).
  For local development use `BUILTIN_THEMES_DIR=../themes` from `backend/`
  to get every theme here.

Zipping one: `cd themes && zip -r nyx2.zip nyx2`.

## Layout

```
nyx2.zip  (or an unzipped nyx2/ folder)
└── theme.json          # at the top, or inside exactly one top-level folder
└── theme.css           #   (GitHub's "Download ZIP" yields nyx2-main/...)
└── fonts/instrument-serif-latin-400-normal.woff2
└── fonts/OFL.txt       # font licence: kept, but not served
```

- Served files: `.css .woff2 .woff .svg .png .jpg .jpeg .webp .avif`, at most
  10 MB together. `theme.css` is required.
- `theme.json` is read, not served. Any other file (`README.md`, `LICENSE`,
  `OFL.txt`, a stray `.scss`) is skipped with a log line.
- Ignored entirely: dotfiles (`.DS_Store`, `._*`) and `__MACOSX/`, which
  macOS Finder adds.
- Symlinks and other non-regular files are skipped; zip entry names must be
  plain relative paths (no `..`).
- The files are read once at startup and served from memory under
  `/theme/<hash>/`, so `url(fonts/x.woff2)` in `theme.css` resolves as is.
  Edit a theme, then restart.

### Self-contained

The Content-Security-Policy allows styles, fonts and images only from the
site itself (and `data:`). External `@import`s and webfonts don't load; put
the font files in the theme. This is deliberate: it stops a theme from
reading the album password through CSS attribute selectors or running script
in an SVG.

## theme.json

```json
{ "smugbox": 1, "name": "Nyx2", "version": "1.0.0", "dark": false, "gallery": "masonry" }
```

| Key | | |
|---|---|---|
| `smugbox` | required | Theme API version. This Smugbox supports `1`. |
| `name` | required | Display name, logged at startup. |
| `version` | optional | Informational, logged at startup. |
| `dark` | default `false` | `true` puts `class="dark"` on `<html>`: shadcn components switch to their dark styles and a pasted `.dark { … }` colour block applies. |
| `gallery` | default `rows` | Album photo layout, see below. |

Unknown keys are rejected; the error says the theme may need a newer
Smugbox. The API version only changes when a token, slot or manifest key is
removed or renamed; additions keep it.

### Gallery layouts

Core owns the react-photo-album props for each layout:

| Layout | Look | Props |
|---|---|---|
| `rows` | justified rows | spacing 12, target row height 320 |
| `columns` | balanced columns, wide gaps | spacing 20, about one column per 300px (1–5) |
| `masonry` | tight brick mosaic | spacing 4, 2 / 3 / 4 columns below 640 / below 1000 / wider |

Padding is 0. The library rounds the container width down to one of 360,
600, 900, 1200, 1536 before laying out, so the masonry thresholds switch at
900 and 1200 container pixels.

## theme.css

Plain CSS, no build step. All of core's CSS sits in cascade layers and
`theme.css` is unlayered, so a theme's rule wins over core's whatever the
selector specificity or load order, without `!important`.

Two hooks: **tokens** (CSS variables on `:root`) for single values, and
**slots** (`[data-slot="…"]` selectors) for everything else.

### Tokens

Set them on `:root`. Anything not set keeps the default below.

**Colours**: exactly shadcn's standard set, defaulting to its neutral
palette (light on `:root`, dark on `.dark`): `--background`, `--foreground`,
`--card`, `--card-foreground`, `--popover`, `--popover-foreground`,
`--primary`, `--primary-foreground`, `--secondary`, `--secondary-foreground`,
`--muted`, `--muted-foreground`, `--accent`, `--accent-foreground`,
`--destructive`, `--border`, `--input`, `--ring`. You can paste the `:root`
/ `.dark` blocks from ui.shadcn.com/themes or tweakcn into `theme.css`; the
`.dark` block applies when `"dark": true`, and the paste's `@theme inline`
block is ignored by browsers. `--chart-*` and `--sidebar-*` are harmless but
unused. Text on the scrim over a photo (album header, locked cover) is white
in every theme.

| Token | Default | Used for |
|---|---|---|
| `--radius` | `0.625rem` | Base corner radius (shadcn) |
| `--r-panel` | `calc(var(--radius) * 1.4)` | Album/folder cards, album header |
| `--r-media` | `calc(var(--radius) * 1.4)` | Covers, header photo |
| `--r-pill` | `9999px` | The top bar |
| `--page-glow` | `none` | `background-image` behind the page |
| `--surface-bg` | `var(--card)` | Frosted/solid surfaces in theme CSS |
| `--surface-border` | `var(--border)` | Lightbox controls and info panel border |
| `--surface-blur` | `0px` | `backdrop-filter` blur of those surfaces |
| `--panel-bg` | `var(--card)` | Lightbox info panel |
| `--control-bg` | `transparent` | Lightbox toolbar buttons |
| `--control-hover-bg` | `var(--accent)` | Lightbox buttons on hover |
| `--lightbox-bg` | `var(--background)` | Lightbox backdrop |
| `--font-sans` | system-ui stack | All text |
| `--font-display` | `var(--font-sans)` | For theme CSS (titles) |
| `--cover-aspect` | `4 / 3` | Album/folder covers |
| `--card-grid-min` | `18rem` | Smallest card width in the auto-fill grid |
| `--card-gap` | `2rem 1.5rem` | Grid gap (row, column) |
| `--reveal-animation` | `fade-in 1s ease-out backwards` | How a cover or photo appears once loaded and in view; core defines `fade-in` and `zoom-fade-in`; `none` makes it just appear |

Tokens inherit, so a theme can scope one to a slot, e.g. photos that zoom in
while covers just appear:

```css
:root { --reveal-animation: zoom-fade-in 1s ease backwards; }
[data-slot="album-card-media"], [data-slot="folder-card-media"] { --reveal-animation: none; }
```

The lightbox also reads the library's own `--yarl__*` variables
(`--yarl__color_button`, `--yarl__slide_captions_container_background`, …),
and theme CSS may target the libraries' documented `yarl__*` and
`react-photo-album--*` classes.

Only the tokens above are the contract. Tailwind's own variables
(`--spacing`, `--text-*`, `--shadow-*`, `--tracking-*`) can be overridden
from `:root` too, but they aren't supported and may change with a Tailwind
upgrade. Beware tweakcn pastes that set `--spacing`: it rescales all of
core's spacing.

### Slots

Title typography has no tokens; style the title slots.

| Slot | Element |
|---|---|
| `page` | The `<main>` page container; set its width and padding here |
| `breadcrumb` | The top bar (`<nav>`), first on every page and in every state. Its first `breadcrumb-item` is the site name; on the home page that item holds a `breadcrumb-page` with shadcn's `font-normal`, so set a font weight on `[data-slot="breadcrumb-item"]:first-child > *` too |
| `child-grid` | Grid of folder and album cards (also while loading); override `grid-template-columns` for fixed columns |
| `album-card`, `folder-card` | The card link |
| `album-card-media`, `folder-card-media` | Cover box. Core marks keyboard focus with an outline; `box-shadow` is left to the theme |
| `album-card-body`, `folder-card-body` | Text below the cover |
| `album-card-title`, `folder-card-title` | Name |
| `album-card-meta` | Photo count and dates |
| `album-hero` | Album header: wraps the photo banner, or is the text header of an album without a cover (`:not(:has([data-slot="album-hero-media"]))`) |
| `album-hero-media` | The banner with the cover photo |
| `album-hero-title`, `album-hero-meta` | Album title, photo count and dates |
| `album-download` | "Download album" button (replaces `button` on that element) |
| `gallery` | Wrapper around the photo layout |
| `photo-info` | Lightbox info panel |
| `password-gate` | Password form of a locked album |
| `empty-state` | "No albums / no photos yet" text |

From the vendored shadcn/ui components: `button`, `input`, `label`,
`badge`, `alert`, `alert-title`, `alert-description`, `skeleton`, `spinner`,
`breadcrumb`, `breadcrumb-list`, `breadcrumb-item`, `breadcrumb-link`,
`breadcrumb-page`, `breadcrumb-separator`.

## Examples

`noir/` is the smallest complete theme: a pasted palette, one font, a sticky
top bar and a few slot rules. `nyx2/` restyles most slots; `glass/` adds
frosted surfaces and SVG badges drawn with pseudo-elements.
