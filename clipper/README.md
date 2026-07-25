# BRUV Clipper

Browser extension (Chrome MV3) that captures social posts into BRUV cards —
and, with one click, appends them as slides to a target slide deck. Twitter/X
is the first supported platform; the architecture is platform-generic (see
`plan/2026-07-25 twitter to slide deck end-to-end.md` for the genericity
contract). Adding a platform = one plugin in `src/lib/plugins/` + one slide
template on the BRUV side.

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

1. Right-click the extension icon → **Options**.
2. Server URL — one of:
   - the **desktop app itself**: set Settings → General → "Local server port"
     to a fixed port (e.g. `9876`), restart the app, then pair to
     `http://127.0.0.1:9876` (without a fixed port the app picks a random
     port each launch, which breaks pairing on restart);
   - an installed **BRUV-Server service**: `http://127.0.0.1:9870`;
   - a remote server, e.g. `http://ripped.tail2ebd58.ts.net:9870`.
3. Bootstrap token: from the server's `bootstrap-token.txt` (same token the
   mobile enrolment QR encodes). Locations: desktop app / dev server →
   `%APPDATA%\bruv\bootstrap-token.txt`; installed Windows service →
   `%PROGRAMDATA%\BRUV\bootstrap-token.txt` (separate config dirs, separate
   tokens).
4. Pair → pick the target repository → optionally pick a category to pin
   clipped cards into (default: Inbox) → Save.

## Use

1. (Once) Click the extension icon → pick or create a **slide deck target**.
2. Browse X. Right-click on a tweet (or a reply, or highlighted text inside
   one) → **Add to BRUV + slide deck**.
3. Repeat for each slide. Every clip: creates a card (text + source link +
   media downloaded into attachments), tags it `twitter`, and appends a
   `post` slide on the `x-post` template to the target deck.
4. Open the deck card in BRUV → **Present**. Boom — usable slide deck.

If the server is unreachable, clips queue locally (media already embedded,
so nothing rots) and drain automatically once it's back — or via **Retry
now** in the popup.

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

## Notes

- The deck slide **binds** its text to the clipped card's Text block — the
  card stays the source of truth; editing the card updates the slide.
- Media is downloaded **at capture time** into BRUV attachments. CDN URLs are
  never the stored reference (they expire/rot).
- The syndication API (`cdn.syndication.twimg.com`) is unofficial; it is
  isolated in `src/lib/plugins/twitter.ts` and only used for video. Breakage
  degrades to poster images.
