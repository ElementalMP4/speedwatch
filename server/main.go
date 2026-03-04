package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const downloadSize = 50 * 1024 * 1024 // 50 MB

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Length", fmt.Sprintf("%d", downloadSize)) // 50MB
	buffer := make([]byte, 1024*1024)                                 // 1MB chunk

	for i := 0; i < downloadSize/len(buffer); i++ {
		_, err := w.Write(buffer)
		if err != nil {
			return
		}
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	bytesReceived, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		http.Error(w, "upload error", http.StatusInternalServerError)
		return
	}
	duration := time.Since(start)

	log.Printf("Received %d bytes in %v\n", bytesReceived, duration)

	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/download", downloadHandler)
	http.HandleFunc("/upload", uploadHandler)

	log.Println("Speed test server running on :9898")
	log.Fatal(http.ListenAndServe(":9898", nil))
}
