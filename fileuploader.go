package main

import (
	"embed"
	_ "embed"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

const maxRequestSize int64 = 100 << 20

//go:embed index.html
var htmlFile embed.FS

func main() {
	var err error

	filepath := ""
	if os.Getenv("FILE_UPLOADER_FILEPATH") != "" {
		filepath = os.Getenv("FILE_UPLOADER_FILEPATH")
	}
	if filepath == "" {
		log.Fatal("FILE_UPLOADER_FILEPATH is required")
	}

	secret := ""
	if os.Getenv("FILE_UPLOADER_SECRET") != "" {
		secret = os.Getenv("FILE_UPLOADER_SECRET")
	}
	if secret == "" {
		log.Fatal("FILE_UPLOADER_SECRET is required")
	}

	port := 0
	if os.Getenv("FILE_UPLOADER_PORT") != "" {
		port, err = strconv.Atoi(os.Getenv("FILE_UPLOADER_PORT"))
		must(err)
	}

	log.Printf("Will write to file %v, using secret %v", filepath, secret)

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServerFS(htmlFile))

	// mut is used to only allow one caller to upload at a time
	var mut sync.Mutex

	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)

		err := r.ParseMultipartForm(maxRequestSize) // allow using a maximum of 100 MiB when parsing
		if err != nil {
			writeResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		clientSecret := r.PostForm.Get("secret")
		if clientSecret != secret {
			writeResponse(w, "incorrect secret", http.StatusUnauthorized)
			return
		}
		file, _, err := r.FormFile("pdf-upload")
		if err != nil {
			writeResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outFile, err := os.Create(filepath)
		if err != nil {
			writeResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer shut(outFile)
		_, err = io.Copy(outFile, file)
		if err != nil {
			writeResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeResponse(w, "File uploaded successfully", http.StatusCreated)
	})
	server := &http.Server{
		Handler:        mux,
		ReadTimeout:    1 * time.Minute,
		WriteTimeout:   1 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	must(err)
	addr := net.JoinHostPort("", strconv.FormatInt(int64(listener.Addr().(*net.TCPAddr).Port), 10))
	log.Printf("Listening on %v", addr)
	must(server.Serve(listener))
}

func writeResponse(w http.ResponseWriter, errMsg string, code int) {
	http.Error(w, errMsg, code)
	log.Printf("Response %v: %v", code, errMsg)
}

// must logs an error and panics. This should only be done for
// startup errors, not after the server is up and running.
func must(err error) {
	if err != nil {
		panic("got a startup error: " + err.Error())
	}
}

func shut(c io.Closer) {
	_ = c.Close()
}
