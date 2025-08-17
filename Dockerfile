# syntax=docker/dockerfile:1.7-labs

# -----------------
# Build image stage
# -----------------
FROM golang:alpine AS build

# Used for setcap below
RUN apk add --no-cache libcap

WORKDIR /build

# Install all the module dependencies early, so that this layer
# can be cached before ranger-ims-go code is copied over.
COPY go.mod go.sum ./
RUN go mod download

COPY ./ ./

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/fileuploader

# Allow app to bind to privileged port numbers
RUN setcap "cap_net_bind_service=+ep" /build/fileuploader


# --------------------
# Deployed image stage
# --------------------
FROM alpine:latest
COPY --from=build /build/fileuploader /app/fileuploader

ENV FILE_UPLOADER_FILEPATH="/tmp/somefile.pdf"
ENV FILE_UPLOADER_SECRET="open sesame"
ENV FILE_UPLOADER_PORT="80"

# Use a non-root user to run the server
USER daemon:daemon

# This should match the IMS_PORT above
EXPOSE 80

CMD [ "/app/fileuploader" ]
