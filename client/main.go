package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	pingTests       = 10
	downloadStreams = 5
	uploadStreams   = 3
	testDuration    = 10 * time.Second
	warmupDuration  = 1 * time.Second
)

var speedTestClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
	},
}

func getEndpoint() string {
	if alt, ok := os.LookupEnv("SPEEDWATCH_ENDPOINT"); ok {
		return alt
	}
	return "https://speedtest.elementalmp4.co.uk"
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
	log.Printf("Using server at %s\n", getEndpoint())

	log.Println("Testing latency...")
	latencyResult, err := measureLatency()
	if err != nil {
		log.Fatalf("Latency test failed: %v", err)
	}

	log.Println("Warming up download...")
	_, err = measureParallelDownload(downloadStreams, warmupDuration)
	if err != nil {
		log.Fatalf("Download warmup failed: %v", err)
	}

	log.Println("Testing download...")
	downloadMbps, err := measureParallelDownload(downloadStreams, testDuration)
	if err != nil {
		log.Fatalf("Download test failed: %v", err)
	}

	log.Println("Warming up upload...")
	_, err = measureParallelUpload(uploadStreams, warmupDuration)
	if err != nil {
		log.Fatalf("Upload warmup failed: %v", err)
	}

	log.Println("Testing upload...")
	uploadMbps, err := measureParallelUpload(uploadStreams, testDuration)
	if err != nil {
		log.Fatalf("Upload test failed: %v", err)
	}

	log.Printf(
		"Latency: %.2fms Loss: %.2f%% Download: %.2fMbps Upload: %.2fMbps\n",
		latencyResult.Latency,
		latencyResult.Loss,
		downloadMbps,
		uploadMbps,
	)
}

func measureLatency() (LatencyResult, error) {
	var result LatencyResult
	var total time.Duration
	var success int

	for i := 0; i < pingTests; i++ {
		start := time.Now()
		resp, err := speedTestClient.Get(getEndpoint() + "/ping")
		if err != nil {
			continue
		}
		resp.Body.Close()
		total += time.Since(start)
		success++
	}

	if success == 0 {
		return result, fmt.Errorf("all pings failed")
	}

	result.Latency = float64(total.Milliseconds()) / float64(success)
	result.Loss = 100 * float64(pingTests-success) / float64(pingTests)

	return result, nil
}

func measureParallelDownload(n int, duration time.Duration) (float64, error) {
	var wg sync.WaitGroup
	results := make(chan TransferResult, n)
	errors := make(chan error, n)
	start := time.Now()

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := measureDownload(duration)
			if err != nil {
				errors <- err
			} else {
				results <- r
			}
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	for err := range errors {
		return 0, err
	}

	var totalBytes float64
	for r := range results {
		totalBytes += r.BytesTransferred
	}

	elapsed := time.Since(start).Seconds()
	if elapsed == 0 {
		return 0, fmt.Errorf("download failed")
	}

	return (totalBytes * 8) / (elapsed * 1024 * 1024), nil
}

func measureDownload(duration time.Duration) (TransferResult, error) {
	var result TransferResult

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", getEndpoint()+"/download", nil)
	if err != nil {
		return result, err
	}

	resp, err := speedTestClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	start := time.Now()
	bytes, _ := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start)

	result.BytesTransferred = float64(bytes)
	result.DurationSeconds = elapsed.Seconds()

	return result, nil
}

type timedReader struct {
	deadline time.Time
}

func (t *timedReader) Read(p []byte) (int, error) {
	if time.Now().After(t.deadline) {
		return 0, io.EOF
	}

	// Fill buffer with zeros
	for i := range p {
		p[i] = 0
	}

	return len(p), nil
}

func measureParallelUpload(n int, duration time.Duration) (float64, error) {
	var wg sync.WaitGroup
	results := make(chan TransferResult, n)
	errors := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := measureUpload(duration)
			if err != nil {
				errors <- err
			} else {
				results <- r
			}
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	for err := range errors {
		return 0, err
	}

	var totalBytes float64
	var totalTime float64

	for r := range results {
		totalBytes += r.BytesTransferred
		totalTime = r.DurationSeconds
	}

	if totalTime == 0 {
		return 0, fmt.Errorf("upload failed")
	}

	return (totalBytes * 8) / (totalTime * 1024 * 1024), nil
}

func measureUpload(duration time.Duration) (TransferResult, error) {
	var result TransferResult

	reader := &timedReader{
		deadline: time.Now().Add(duration),
	}

	req, err := http.NewRequest(
		"POST",
		getEndpoint()+"/upload",
		reader,
	)

	if err != nil {
		return result, err
	}

	req.ContentLength = -1

	start := time.Now()
	resp, err := speedTestClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return result, err
	}

	defer resp.Body.Close()

	var uploadResponse struct {
		Bytes   int64   `json:"bytes"`
		Seconds float64 `json:"seconds"`
	}

	if decodeErr := json.NewDecoder(resp.Body).Decode(&uploadResponse); decodeErr != nil {
		return result, decodeErr
	}

	result.BytesTransferred = float64(uploadResponse.Bytes)
	result.DurationSeconds = elapsed.Seconds()

	return result, nil
}
