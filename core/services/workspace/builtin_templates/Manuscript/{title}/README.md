# {title}

A BRUV manuscript folder. Plain Markdown by convention (Pandoc's), nothing proprietary — every other Markdown tool keeps working on these files.

- `manuscript.yaml` — title, author and the ordered list of parts. Only listed files are part of the book.
- `chapters/` — one file per chapter. The first `# Heading` is the chapter title; `* * *` on its own line is a scene break.
- `front-matter/`, `back-matter/` — copyright, dedication, about the author. Small files with a `role` in the manifest.
- Prose uses `*emphasis*` only; straight quotes and `--` become curly quotes and dashes at export, so type them plainly.

Open any file from the Workspace panel to write with a live preview and outline.
