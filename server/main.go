package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type Config struct {
	ServerAddress string `json:"bind"`
}

func loadConfig() (Config, error) {
	var config Config
	data, err := os.ReadFile("config.json")
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(data, &config)
	return config, err
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("x-speedwatch-detect", "Hello from SpeedWatch!")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("x-speedwatch-detect", "Hello from SpeedWatch!")

	// 32KB buffer, which we re-use for efficiency
	buffer := make([]byte, 32*1024)
	flusher, _ := w.(http.Flusher)

	for {
		_, err := w.Write(buffer)
		if err != nil {
			// Client disconnected
			return
		}

		if flusher != nil {
			flusher.Flush()
		}
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	start := time.Now()

	bytesReceived, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		log.Printf("Upload error: %v\n", err)
		http.Error(w, "upload error", http.StatusInternalServerError)
		return
	}

	duration := time.Since(start)

	log.Printf("Received %d bytes in %v\n", bytesReceived, duration)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("x-speedwatch-detect", "Hello from SpeedWatch!")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"bytes":   bytesReceived,
		"seconds": duration.Seconds(),
	})
}

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", pingHandler)
	mux.HandleFunc("/download", downloadHandler)
	mux.HandleFunc("/upload", uploadHandler)

	server := &http.Server{
		Addr:              config.ServerAddress,
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Speed test server running on %s\n", config.ServerAddress)
	log.Fatal(server.ListenAndServe())
}
