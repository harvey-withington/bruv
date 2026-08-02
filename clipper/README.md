# BRUV Clipper

Browser extension (Chrome MV3) that captures social posts into BRUV cards —
and, with one click, appends them as slides to a target slide deck.
Supported platforms: **Twitter/X, Truth Social, Reddit, YouTube** — one
plugin each in `src/lib/plugins/` (see
`plan/2026-07-25 twitter to slide deck end-to-end.md` for the genericity
contract). Adding a platform = one plugin file + a registry line; plugins
know nothing about slide templates — slides are stamped `auto` and BRUV
resolves the template from the capture URL (per-platform look, retroactive
upgrades; see `plan/2026-07-31 per-platform slide templates and auto
matching.md`).

## Build

```powershell
cd clipper
npm install
npm run check   # tsc --noEmit
npm run build   # bundles into dist/
```

## Install (dev / sideload)

1. Chrome → `chrome://extensions` → enable **Developer mode**.
2. **Load unpacked** → select `clipper/dist/`.

## Pair with your BRUV server

Open the popup → **Options** (top-right; always available).

### A server on this machine — no token needed

1. Click **Find the server on this machine**. It probes loopback ports
   9870-9879 and identifies a real BRUV server by its `/version` response,
   then fills the URL in. (The desktop app defaults to **9870** and, if that
   port is taken, moves within that range and tells you which port it got.
   Settings → General → "Local server port" overrides the default.)
2. Click **Pair automatically** — no bootstrap token. The server accepts
   token-less pairing only for unproxied loopback requests from a
   browser-privileged origin, which is the same trust the desktop app already
   places in reading its own token file. See `transport/http/localpair.go`.
3. Pick the target repository, and where clipped cards should be pinned
   (default: Inbox) → **Save**.

### A remote server — bootstrap token

Locality is the credential above, and a remote request hasn't got one, so
remote pairing still uses the token:

1. Enter the server URL, e.g. `https://myserver.tailnet.ts.net`.
2. Paste the bootstrap token from `bootstrap-token.txt` on that server — the
   same token the mobile enrolment QR encodes. Locations: desktop app / dev
   server → `%APPDATA%\bruv\bootstrap-token.txt`; installed Windows service →
   `%PROGRAMDATA%\BRUV\bootstrap-token.txt` (separate config dirs, separate
   tokens). The server also serves a pairing page with a QR at
   `/pair?token=<token>`.
3. Pair → pick the repository and pin target → **Save**.

Pair the extension to the **same server your phone shares to** — pending
clips (below) only appear for the paired server.

## Use

1. (Once) Open the popup → type in the **deck target** box to search your
   cards (recents appear on focus) and pick a deck — or create one with
   **New deck**. The × inside the box clears it.
2. Browse a supported site. Right-click a post — or a reply, or text you've
   highlighted inside one → **Add to BRUV + slide deck**.
3. Repeat for each slide. Every clip creates a card (author, handle, avatar,
   text, date, source link, media downloaded into attachments), tags it with
   the platform, and appends a `post` slide to the target deck. The slide
   binds to the card's blocks, so editing the card updates the slide; the
   template resolves from the capture URL at render time.
4. Open the deck card in BRUV → **Present**. Boom — usable slide deck.

If the server is unreachable, clips queue locally (media already embedded,
so nothing rots) and drain automatically once it's back — or via **Retry
now** in the popup. While the server is down the popup greys out what it
can't do, rather than pretending.

## Capture options (the dialog)

Capture decisions are yours, made at capture time and pre-filled from your
vault's capture defaults (BRUV → Settings → Capture; they live in the vault,
so the phone and this extension agree). Design:
`plan/2026-08-02 capture options at capture time.md`.

**Right-click → "Add to BRUV (options…)"** always shows the dialog. The other
two menu items show it only when your own triggers say the decision is
consequential — an oversized video, a gallery over N images, a platform that
blocks BRUV's server — and capture silently otherwise. Set
Settings → Capture → "Show the capture dialog" to *always* or *never* to move
that line. (It's a third menu item rather than Shift+click because Chrome's
context-menu events don't report modifier keys.)

The dialog offers:

- **Title** — pre-filled, editable.
- **Video** — every quality as a real choice, each with its estimated size
  (`1280×720 · ~725 MB`), plus *Link only* and *Skip*. The ladder comes from
  the plugin when it has one (X's syndication API) and from the server's
  `PreviewCapture` otherwise. Downloading happens **here**, in your
  logged-in browser — so "store the 3.5 GB rung" means exactly that.
- **Images** — all / first only / link only / skip.
- **Destination** — the deck and pin targets shown read-only (they're sticky
  settings: deck in the popup, pin in Options), plus a live *Add a slide to
  this deck* checkbox for this capture.

Honesty rules: when BRUV's server can't read the URL (X blocks it) or has no
reader for the site, the dialog says so — the capture still runs from the
page in front of you, but the size estimates then come from the page alone.
Escape or a click outside cancels and nothing is written.

## Pending clips (completing a phone capture)

When you share a URL to BRUV from your phone and the platform blocks the
server from reading it (Reddit does this to everyone now), the server still
creates the card, the source link and the deck slide, and marks the card
`clip-pending`. This extension finishes the job, because it runs in a real
browser that's already logged in.

- The toolbar icon shows a **badge** with the number of clips waiting.
- The popup lists them under **Pending clips**. **Complete** opens the post
  in a tab, captures it with the same plugins, fills the card's blocks in
  place — so the slide that's already in your deck fills in where it stands —
  and closes the tab. Completions run one at a time in the background, so
  "Complete all" keeps working after the popup closes (opening a tab takes
  focus, which closes it).
- The red **×** deletes a pending clip you'll never complete (a dead link,
  say). It arms on the first click and deletes on the second. Note it deletes
  the *card*; a slide already added for it stays in the deck.

## Manual test checklist (needs a live browser on real X pages)

- [ ] Pair against the local desktop server; repo + category pickers populate.
- [ ] Clip a **text-only tweet** from the timeline → card appears with Text +
      Source blocks, tag `twitter`.
- [ ] Clip a **reply** from inside a thread → the card's Source link is the
      reply's own permalink, not the thread root.
- [ ] Clip an **image tweet** → image lands as a card attachment; the slide's
      media shows it (attachment-signed, not a CDN link).
- [ ] Clip a **video tweet** → mp4 resolved via the syndication API; if that
      fails, the slide shows the poster image instead (clip must not fail).
- [ ] **Highlight a sentence** inside a long tweet → clip → the slide/card
      text is just the highlighted passage.
- [ ] Set a deck target once → second clip is a single right-click (no picker).
- [ ] "Add to BRUV + slide deck" with no target set → in-page toast explains,
      nothing half-created.
- [ ] Stop the server → clip → "queued" toast; restart server → popup Retry
      (or wait a minute) → card + slide appear.
- [ ] Present the deck → slides render on the x-post template with avatar,
      name, @handle, text, media, date.

### Capture options dialog

- [ ] Clip a **video tweet** with defaults (ask on video over 50 MB) → the
      dialog appears listing every quality with a size; Capture stores the
      pre-selected one.
- [ ] Pick the **largest** rung on a long video → it downloads (slowly) and
      lands as an attachment, not a poster image.
- [ ] Pick **Link only** → the card's Video block holds the platform URL and
      no video attachment is created; the slide still plays.
- [ ] Pick **Skip** → no video at all, and the rest of the card is intact.
- [ ] Clip a **gallery** over the gallery trigger → dialog says how many
      images; *First image only* attaches exactly one.
- [ ] Edit the **title** → the created card uses the edited title.
- [ ] **Escape**, a click on the backdrop, and **Cancel** each close the
      dialog and create nothing (toast says so).
- [ ] Keystrokes in the title field don't trigger the host page's shortcuts
      (test on x.com, which binds single-key shortcuts).
- [ ] The dialog looks right on a light-themed site and on X — no inherited
      page styling, nothing clipped off-screen at small window sizes.
- [ ] "Add to BRUV (options…)" on a **text-only** post → dialog shows title
      and destination only (no video/images sections).
- [ ] Leave the dialog open for a few minutes, then Capture → it still lands
      (the content script pings the service worker to keep it alive).
- [ ] Settings → Capture → "never" → right-click capture never shows the
      dialog and applies the defaults.
- [ ] Stop the server → "Add to BRUV (options…)" → the dialog still appears
      (defaults, no size estimates) and the clip queues after Capture.

### Truth Social

- [ ] Capture a plain text post — author, handle, and text populate
      correctly after API enrichment.
- [ ] Capture a post with a single image — image lands as an attachment,
      not a blank/rotting CDN link.
- [ ] Capture a post with video — resolves to a playable mp4 via the
      statuses API (or degrades cleanly to the poster image).
- [ ] Capture a reply in a thread — canonicalUrl is the reply's own
      permalink, not the thread root.

### Reddit

- [ ] New Reddit (www): capture a text/self post — title + truncated
      selftext preview land on card and slide.
- [ ] New Reddit: capture a gallery post — every image attaches (cap 12);
      the slide shows them as a carousel (counter badge; console Images
      button / click-in-preview advances, wrapping at the end).
- [ ] New Reddit: capture a native video post — video attaches (video-only,
      no audio track — accepted trade-off) with poster fallback.
- [ ] old.reddit.com: capture via div.thing — author/subreddit/title match
      the new-Reddit capture of the same post.

### YouTube

- [ ] Right-click on a watch page (away from thumbnails) — captures that
      video with title, channel, @handle, avatar.
- [ ] Right-click a thumbnail on home/search — captures that video's id;
      title, channel name, and @handle fill via the oEmbed lookup (only the
      channel avatar stays blank — oEmbed doesn't carry it).
- [ ] Shorts: thumbnail and watch page both resolve the right video id.
- [ ] Old/low-res video (no maxresdefault) — thumbnail downgrades to
      hqdefault, not a broken image.
- [ ] Slide renders the official YouTube embed (iframe) at Present time;
      the card holds the thumbnail + source link (no video download).

## Notes

- The deck slide **binds** its text to the clipped card's Text block — the
  card stays the source of truth; editing the card updates the slide.
- Media is downloaded **at capture time** into BRUV attachments. CDN URLs are
  never the stored reference (they expire/rot).
- The syndication API (`cdn.syndication.twimg.com`) is unofficial; it is
  isolated in `src/lib/plugins/twitter.ts` and only used for video. Breakage
  degrades to poster images.
