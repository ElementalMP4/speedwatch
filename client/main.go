package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func getEndpoint() string {
	alternativeEndpoint, overridden := os.LookupEnv("SPEEDWATCH_ENDPOINT")
	if overridden {
		return alternativeEndpoint
	} else {
		return "http://10.43.237.1:9898"
	}
}

type LatencyResult struct {
	Latency float64
	Loss    float64
}

type TransferResult struct {
	BytesTransferred float64
	DurationSeconds  float64
}

func (t *TransferResult) CalculateSpeed() float64 {
	return (t.BytesTransferred * 8) / (t.DurationSeconds * 1024 * 1024)
}

func main() {
	log.Println("Testing latency...")

	latencyResult, err := measureLatency()
	if err != nil {
		log.Fatalf("Latency test failed: %v", err)
	}

	log.Println("Testing download...")
	downloadResult, err := measureDownload()
	if err != nil {
		log.Fatalf("Download test failed: %v", err)
	}

	log.Println("Testing upload...")
	uploadResult, err := measureUpload()
	if err != nil {
		log.Fatalf("Upload test failed: %v", err)
	}

	log.Printf("Latency: %fms Loss: %f%% Download: %fMbps Upload: %fMbps",
		latencyResult.Latency, latencyResult.Loss, downloadResult.CalculateSpeed(), uploadResult.CalculateSpeed())
}

func measureLatency() (LatencyResult, error) {
	var result LatencyResult
	var total time.Duration
	var success int
	var errors []error
	client := &http.Client{Timeout: 3 * time.Second}
	attempts := 5

	for i := 0; i < attempts; i++ {
		start := time.Now()
		resp, err := client.Get(getEndpoint() + "/ping")
		if err != nil {
			errors = append(errors, err)
			continue
		}
		resp.Body.Close()
		total += time.Since(start)
		success++
	}

	if success == 0 {
		var errorStrings []string

		for _, err := range errors {
			errorStrings = append(errorStrings, err.Error())
		}
		return result, fmt.Errorf("all pings failed: %s", strings.Join(errorStrings, ", "))
	}

	result.Latency = float64(total.Milliseconds()) / float64(success)
	result.Loss = 100 * float64(attempts-success) / float64(attempts)

	return result, nil
}

func measureDownload() (TransferResult, error) {
	var result TransferResult
	client := &http.Client{Timeout: 30 * time.Second}
	start := time.Now()
	resp, err := client.Get(getEndpoint() + "/download")
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	bytes, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return result, err
	}

	result.BytesTransferred = float64(bytes)
	result.DurationSeconds = time.Since(start).Seconds()

	return result, nil
}

func measureUpload() (TransferResult, error) {
	var result TransferResult
	size := 10 * 1024 * 1024 // 10MB
	data := make([]byte, size)

	client := &http.Client{Timeout: 30 * time.Second}
	start := time.Now()
	resp, err := client.Post(getEndpoint()+"/upload",
		"application/octet-stream",
		bytes.NewReader(data))
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	result.BytesTransferred = float64(size)
	result.DurationSeconds = time.Since(start).Seconds()

	return result, nil
}
