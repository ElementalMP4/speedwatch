package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const downloadSize = 10 * 1024 * 1024 // 10 MB

type Config struct {
	ServerAddress string `json:"bind"`
}

func loadConfig() (Config, error) {
	var config Config
	data, err := os.ReadFile("config.json")
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}

	return config, nil
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Length", fmt.Sprintf("%d", downloadSize))
	buffer := make([]byte, downloadSize)
	_, err := w.Write(buffer)
	if err != nil {
		log.Printf("Error whilst sending download: %v\n", err)
		return
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	bytesReceived, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		log.Printf("Error whilst receiving upload: %v\n", err)
		http.Error(w, "upload error", http.StatusInternalServerError)
		return
	}
	duration := time.Since(start)

	log.Printf("Received %d bytes in %v\n", bytesReceived, duration)
	w.WriteHeader(http.StatusOK)
}

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/download", downloadHandler)
	http.HandleFunc("/upload", uploadHandler)

	log.Printf("Speed test server running on %s\n", config.ServerAddress)
	log.Fatal(http.ListenAndServe(config.ServerAddress, nil))
}
