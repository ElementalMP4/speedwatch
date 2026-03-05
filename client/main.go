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
	return "http://10.43.237.1:9898"
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
	measureParallelDownload(downloadStreams, warmupDuration)

	log.Println("Testing download...")
	downloadMbps, err := measureParallelDownload(downloadStreams, testDuration)
	if err != nil {
		log.Fatalf("Download test failed: %v", err)
	}

	log.Println("Warming up upload...")
	measureParallelUpload(uploadStreams, warmupDuration)

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
	attempts := 5

	for i := 0; i < attempts; i++ {
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
	result.Loss = 100 * float64(attempts-success) / float64(attempts)

	return result, nil
}

func measureParallelDownload(n int, duration time.Duration) (float64, error) {
	var wg sync.WaitGroup
	results := make(chan TransferResult, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _ := measureDownload(duration)
			results <- r
		}()
	}

	wg.Wait()
	close(results)

	var totalBytes float64
	var totalTime float64

	for r := range results {
		totalBytes += r.BytesTransferred
		totalTime += r.DurationSeconds
	}

	totalTime = totalTime / float64(len(results))

	if totalTime == 0 {
		return 0, fmt.Errorf("download failed")
	}

	return (totalBytes * 8) / (totalTime * 1024 * 1024), nil
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

type zeroReader struct{}

func (z zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func measureParallelUpload(n int, duration time.Duration) (float64, error) {
	var wg sync.WaitGroup
	results := make(chan TransferResult, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _ := measureUpload(duration)
			results <- r
		}()
	}

	wg.Wait()
	close(results)

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

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		getEndpoint()+"/upload",
		zeroReader{},
	)
	if err != nil {
		return result, err
	}

	// Force chunked encoding (infinite stream)
	req.ContentLength = -1

	start := time.Now()
	resp, err := speedTestClient.Do(req)
	elapsed := time.Since(start)

	if ctx.Err() == context.DeadlineExceeded {
		err = nil
	}

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
