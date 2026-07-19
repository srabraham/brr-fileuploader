# brr-fileuploader

A tiny web server that hosts **one** PDF and lets you replace it over the course of
the week.

It runs on a server on playa. Dashboards elsewhere on playa point at its `/payload`
URL and re-fetch the PDF from time to time, so re-uploading a new version is all it
takes to update what those dashboards display. There's no build step, no database,
and no accounts — just a form, a shared secret, and a file.

## How it works

Three URLs:

| URL | What it does |
| --- | --- |
| `/index.html` | Upload form. Pick a PDF, type the secret, submit. |
| `/payload` | The current PDF, served raw. This is what the dashboards fetch. |
| `/messageboard.html` | Renders the current PDF in the browser, for eyeballing what's live. |

Uploading overwrites the previous file — there is only ever one, and no history is
kept. Uploads are limited to 100 MiB, and only one upload is processed at a time.

The secret protects uploads only. `/payload` is deliberately open so any dashboard
can read it without credentials.

## Configuration

Set via environment variables:

| Variable | Required | Notes |
| --- | --- | --- |
| `FILE_UPLOADER_FILEPATH` | yes | Where the PDF is stored on disk. |
| `FILE_UPLOADER_SECRET` | yes | Shared secret people type into the upload form. |
| `FILE_UPLOADER_PORT` | no | Defaults to an OS-assigned free port, which is printed to the log at startup. |

## Running it

On playa this runs under Docker:

```bash
docker build -t brr-fileuploader .
docker run -d --restart unless-stopped \
  -p 80:80 \
  -v /srv/brr-fileuploader:/data \
  -e FILE_UPLOADER_FILEPATH=/data/payload.pdf \
  -e FILE_UPLOADER_SECRET='pick-something-real' \
  brr-fileuploader
```

Two things worth getting right before you drive out:

- **Mount a volume.** The Dockerfile defaults to `/tmp/somefile.pdf` *inside* the
  container, so the uploaded PDF disappears if the container is recreated. Pointing
  `FILE_UPLOADER_FILEPATH` at a mounted directory (as above) means the file survives
  restarts and reboots.
- **Override the secret.** The Dockerfile ships with `open sesame` as a placeholder.

For local development you can skip Docker:

```bash
FILE_UPLOADER_FILEPATH=/tmp/somefile.pdf \
FILE_UPLOADER_SECRET='open sesame' \
FILE_UPLOADER_PORT=8080 \
go run .
```

Then open <http://localhost:8080/index.html>.

Note that the server logs the secret in plaintext at startup, so treat the logs with
the same care as the secret itself.

## Updating the file during the week

Open `/index.html`, choose the new PDF, enter the secret, submit. You should get
`File uploaded successfully`. Load `/messageboard.html` to confirm the right document
is live; the dashboards will pick it up on their next fetch.
