// Screenplay layout for the preview pane and for print. US Letter, Courier
// 12pt, industry margins: 1.5in left, 1in elsewhere; cue at 3.7in from
// the page edge (2.2in into the text block), dialogue 2.5in–6in,
// parentheticals 3.1in–5.5in. Screen shows the same geometry on a page-
// shaped sheet; the writer's own notes, sections and synopses show muted
// on screen and vanish in print. All colours are design tokens.
export const fountainStyles = `
.doc-preview.fountain {
  max-width: none;
  font-family: "Courier Prime", "Courier New", Courier, monospace;
  font-size: 12pt;
  line-height: 1;
  color: var(--text-primary);
}
.doc-preview.fountain .fn-title-page,
.doc-preview.fountain .fn-script {
  box-sizing: border-box;
  width: 8.5in;
  max-width: 100%;
  margin: 0 auto 1.5rem;
  padding: 1in 1in 1in 1.5in;
  background: var(--bg-elevated);
  border: 1px solid var(--border-muted);
}
.doc-preview.fountain .fn-title-page {
  min-height: 11in;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  page-break-after: always;
}
.doc-preview.fountain .fn-tp-center { text-align: center; margin-top: 3in; }
.doc-preview.fountain .fn-tp { margin: 0 0 1em; white-space: pre-wrap; }
.doc-preview.fountain .fn-tp-title { text-transform: uppercase; text-decoration: underline; margin-bottom: 2em; }
.doc-preview.fountain .fn-tp-corner { margin-top: auto; }
.doc-preview.fountain p { margin: 0 0 1em; white-space: pre-wrap; }
.doc-preview.fountain .fn-scene-heading {
  text-transform: uppercase;
  font-weight: 700;
  margin-top: 2em;
  display: flex;
  justify-content: space-between;
  gap: 1em;
}
.doc-preview.fountain .fn-scene-number { font-weight: 400; }
.doc-preview.fountain .fn-action { width: 6in; max-width: 100%; }
.doc-preview.fountain .fn-dialogue-block { margin: 0 0 1em; page-break-inside: avoid; }
.doc-preview.fountain .fn-character { margin: 0 0 0 2.2in; text-transform: uppercase; }
.doc-preview.fountain .fn-dialogue { margin: 0 0 0 1in; width: 3.5in; max-width: calc(100% - 1in); }
.doc-preview.fountain .fn-parenthetical { margin: 0 0 0 1.6in; width: 2.5in; max-width: calc(100% - 1.6in); }
.doc-preview.fountain .fn-lyric { font-style: italic; }
.doc-preview.fountain .fn-transition { text-align: right; text-transform: uppercase; }
.doc-preview.fountain .fn-centered { text-align: center; }
.doc-preview.fountain .fn-page-break {
  border: none;
  border-top: 1px dashed var(--border);
  margin: 2em 0;
  page-break-after: always;
}
.doc-preview.fountain .fn-dual { display: flex; gap: 0.25in; }
.doc-preview.fountain .fn-dual .fn-dialogue-block { flex: 1; min-width: 0; }
.doc-preview.fountain .fn-dual .fn-character { margin-left: 1in; }
.doc-preview.fountain .fn-dual .fn-dialogue { margin-left: 0; width: auto; max-width: none; }
.doc-preview.fountain .fn-dual .fn-parenthetical { margin-left: 0.5in; width: auto; max-width: none; }
.doc-preview.fountain .fn-section { color: var(--text-muted); font-weight: 700; margin-top: 1.5em; }
.doc-preview.fountain .fn-section-1 { font-size: 1.15em; }
.doc-preview.fountain .fn-synopsis { color: var(--text-muted); font-style: italic; }
.doc-preview.fountain .fn-note, .doc-preview.fountain .fn-note-block { color: var(--text-muted); font-style: italic; }
@media print {
  .doc-preview.fountain { color: #000; }
  .doc-preview.fountain .fn-title-page, .doc-preview.fountain .fn-script {
    width: auto; margin: 0; padding: 0; background: none; border: none;
  }
  .doc-preview.fountain .fn-page-break { border: none; margin: 0; }
  .doc-preview.fountain .fn-section, .doc-preview.fountain .fn-synopsis,
  .doc-preview.fountain .fn-note, .doc-preview.fountain .fn-note-block { display: none; }
  /* Screenplay page numbers: top right, followed by a period, none on the
     title/first page. Chromium honours @page margin boxes from 131; the
     print dialog's own "Headers and footers" must be off or it draws its
     date/URL header as well. */
  @page {
    size: letter;
    margin: 1in 1in 1in 1.5in;
    @top-right { content: counter(page) "."; font-family: "Courier Prime", "Courier New", Courier, monospace; font-size: 12pt; vertical-align: bottom; padding-bottom: 0.5in; }
  }
  @page :first { @top-right { content: none; } }
}
`
