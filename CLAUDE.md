# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o fileuploader

# Run locally (all three env vars are read at startup; the first two are mandatory)
FILE_UPLOADER_FILEPATH=/tmp/somefile.pdf FILE_UPLOADER_SECRET='open sesame' FILE_UPLOADER_PORT=8080 go run .

# Docker
docker build -t brr-fileuploader .
docker run -p 8080:80 brr-fileuploader

# Tests (none exist yet)
go test ./...
go test -run TestName ./...
```

`FILE_UPLOADER_PORT` is optional — unset or empty means port 0, i.e. the OS picks a
free port, which the server then logs.

## Architecture

Single-binary Go HTTP server (`fileuploader.go`, ~120 lines) plus a `web/` package
whose only job is to `//go:embed *` the static assets into the binary. There are no
third-party Go dependencies (`go.sum` is empty), so the whole app compiles and ships
as one static binary.

Three routes, all registered in `main`:

- `/` — serves `web/` from the embedded FS. `index.html` is the upload form;
  `messageboard.html` is the viewer, which renders `/payload` with the vendored
  PDF.js (`pdf.mjs`, `pdf.worker.mjs`, `pdfjs.js`).
- `/upload` — multipart POST. Form fields are `secret` and `pdf-upload`.
- `/payload` — serves the single uploaded file back.

The server holds exactly **one** file at a time: every upload overwrites the path in
`FILE_UPLOADER_FILEPATH`. `uploadHandler` closes over a `sync.Mutex` held for the
whole request so concurrent uploads can't interleave writes to that one path. Uploads
are capped at 100 MiB by both `http.MaxBytesReader` and `ParseMultipartForm`.

Auth is a single shared secret compared against the `secret` form field — there are no
sessions or users. The secret gates `/upload` only: `/payload` is intentionally public
so anyone can read the current PDF. Don't "fix" that by adding auth to `/payload`.

`must()` is for startup errors only (it panics); anything after the server is up should
go through `writeResponse`, which both writes the HTTP error and logs it.

## Conventions

- The PDF.js files in `web/` are vendored third-party code — don't reformat or lint them.
