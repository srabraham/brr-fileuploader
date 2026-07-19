package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// multipartUpload builds the multipart body the upload form sends. A field is
// omitted entirely when its argument is nil.
func multipartUpload(t *testing.T, secret *string, fileContents []byte) (body *bytes.Buffer, contentType string) {
	t.Helper()

	body = &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if secret != nil {
		if err := w.WriteField("secret", *secret); err != nil {
			t.Fatalf("WriteField(secret): %v", err)
		}
	}
	if fileContents != nil {
		part, err := w.CreateFormFile("pdf-upload", "doc.pdf")
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := part.Write(fileContents); err != nil {
			t.Fatalf("writing file part: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}
	return body, w.FormDataContentType()
}

func TestUploadSucceeds(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "payload.pdf")
	body, contentType := multipartUpload(t, new("open sesame"), []byte("%PDF-1.7 hello playa"))

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	uploadHandler("open sesame", dest)(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d (body: %q)", rec.Code, http.StatusCreated, rec.Body.String())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading uploaded file: %v", err)
	}
	if string(got) != "%PDF-1.7 hello playa" {
		t.Errorf("file contents = %q, want %q", got, "%PDF-1.7 hello playa")
	}
}

func TestUploadOverwritesPreviousFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "payload.pdf")
	if err := os.WriteFile(dest, []byte("monday's schedule"), 0o600); err != nil {
		t.Fatalf("seeding existing file: %v", err)
	}
	body, contentType := multipartUpload(t, new("open sesame"), []byte("thursday's schedule"))

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	uploadHandler("open sesame", dest)(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d (body: %q)", rec.Code, http.StatusCreated, rec.Body.String())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading uploaded file: %v", err)
	}
	if string(got) != "thursday's schedule" {
		t.Errorf("file contents = %q, want the new upload to have replaced the old one", got)
	}
}

func TestUploadWithWrongSecretIsRejectedAndLeavesFileAlone(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "payload.pdf")
	if err := os.WriteFile(dest, []byte("the good file"), 0o600); err != nil {
		t.Fatalf("seeding existing file: %v", err)
	}
	body, contentType := multipartUpload(t, new("wrong"), []byte("the bad file"))

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	uploadHandler("open sesame", dest)(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "incorrect secret") {
		t.Errorf("body = %q, want it to mention the incorrect secret", rec.Body.String())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(got) != "the good file" {
		t.Errorf("file contents = %q, want the rejected upload to have left it untouched", got)
	}
}

func TestUploadWithNoSecretFieldIsRejected(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "payload.pdf")
	body, contentType := multipartUpload(t, nil, []byte("%PDF-1.7"))

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	uploadHandler("open sesame", dest)(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("os.Stat(dest) err = %v, want the file never to have been created", err)
	}
}

func TestUploadWithNoFileFieldFails(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "payload.pdf")
	body, contentType := multipartUpload(t, new("open sesame"), nil)

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	uploadHandler("open sesame", dest)(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("os.Stat(dest) err = %v, want the file never to have been created", err)
	}
}

func TestUploadWithNonMultipartBodyFails(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "payload.pdf")

	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("secret=open+sesame"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	uploadHandler("open sesame", dest)(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// Concurrent uploads are serialized by the handler's mutex, so the file on disk
// must always be exactly one of the uploads rather than a mix of both.
func TestConcurrentUploadsDoNotInterleave(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "payload.pdf")
	handler := uploadHandler("open sesame", dest)

	first := bytes.Repeat([]byte("a"), 64<<10)
	second := bytes.Repeat([]byte("b"), 64<<10)

	var wg sync.WaitGroup
	for _, contents := range [][]byte{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, contentType := multipartUpload(t, new("open sesame"), contents)
			req := httptest.NewRequest(http.MethodPost, "/upload", body)
			req.Header.Set("Content-Type", contentType)
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != http.StatusCreated {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
			}
		}()
	}
	wg.Wait()

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading uploaded file: %v", err)
	}
	if !bytes.Equal(got, first) && !bytes.Equal(got, second) {
		t.Errorf("file is %d bytes and matches neither upload; the two writes interleaved", len(got))
	}
}

func TestOversizedUploadIsRejected(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "payload.pdf")

	// A body that claims to be multipart but streams more than maxRequestSize.
	req := httptest.NewRequest(http.MethodPost, "/upload", io.LimitReader(neverEndingReader{}, maxRequestSize+(1<<20)))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	rec := httptest.NewRecorder()

	uploadHandler("open sesame", dest)(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("os.Stat(dest) err = %v, want the file never to have been created", err)
	}
}

type neverEndingReader struct{}

func (neverEndingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}
