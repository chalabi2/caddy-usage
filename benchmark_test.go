//go:build !integration
// +build !integration

// Package caddyusage provides benchmarks for the Caddy HTTP usage collector module.
package caddyusage

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// BenchmarkClientIPExtraction benchmarks the client IP extraction function
// Category: Performance Tests - IP extraction performance
func BenchmarkClientIPExtraction(b *testing.B) {
	req := &http.Request{
		RemoteAddr: "192.168.1.100:8080",
		Header:     make(http.Header),
	}
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1, 172.16.0.1")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = getClientIP(req)
	}
}

// BenchmarkHeaderMetricsCollection benchmarks header processing
func BenchmarkHeaderMetricsCollection(b *testing.B) {
	registry := prometheus.NewRegistry()
	uc := &UsageCollector{
		logger:   zap.NewNop(),
		registry: registry,
	}

	err := uc.registerMetrics()
	if err != nil {
		b.Fatalf("Failed to register metrics: %v", err)
	}

	req := &http.Request{
		Header: make(http.Header),
	}
	req.Header.Set("User-Agent", "benchmark-client/1.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Content-Type", "application/json")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		uc.collectHeaderMetrics(req, "GET", "200")
	}
}

// BenchmarkMetricsRegistration benchmarks the metrics registration process
func BenchmarkMetricsRegistration(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		registry := prometheus.NewRegistry()
		uc := &UsageCollector{
			logger:   zap.NewNop(),
			registry: registry,
		}

		err := uc.registerMetrics()
		if err != nil {
			b.Fatalf("Failed to register metrics: %v", err)
		}
	}
}

// BenchmarkConcurrentMetricsCollection benchmarks concurrent metric recording
func BenchmarkConcurrentMetricsCollection(b *testing.B) {
	registry := prometheus.NewRegistry()
	uc := &UsageCollector{
		logger:   zap.NewNop(),
		registry: registry,
	}

	err := uc.registerMetrics()
	if err != nil {
		b.Fatalf("Failed to register metrics: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			uc.requestsTotal.WithLabelValues("200", "GET", "benchmark.test", "/test").Inc()
			uc.requestsByIP.WithLabelValues("192.168.1.1", "200", "GET").Inc()
			uc.requestDuration.WithLabelValues("GET", "200", "benchmark.test").Observe(0.1)
		}
	})
}

// BenchmarkDifferentURLPatterns benchmarks with different URL patterns
func BenchmarkDifferentURLPatterns(b *testing.B) {
	registry := prometheus.NewRegistry()
	uc := &UsageCollector{
		logger:   zap.NewNop(),
		registry: registry,
	}

	err := uc.registerMetrics()
	if err != nil {
		b.Fatalf("Failed to register metrics: %v", err)
	}

	// Different URL patterns to test
	urlPatterns := []string{
		"/api/users",
		"/api/users/123",
		"/api/posts?page=1&limit=10",
		"/static/css/style.css",
		"/uploads/images/photo.jpg",
		"/api/search?q=golang&type=code&sort=updated",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		urlPattern := urlPatterns[i%len(urlPatterns)]
		clientIP := "192.168.1." + strconv.Itoa(i%255)
		uc.requestsByURL.WithLabelValues(urlPattern, "GET", "200").Inc()
		uc.requestsByIP.WithLabelValues(clientIP, "200", "GET").Inc()
	}
}

// BenchmarkMemoryUsage tests memory allocation patterns
func BenchmarkMemoryUsage(b *testing.B) {
	registry := prometheus.NewRegistry()
	uc := &UsageCollector{
		logger:   zap.NewNop(),
		registry: registry,
	}

	err := uc.registerMetrics()
	if err != nil {
		b.Fatalf("Failed to register metrics: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Setup request data
		method := "POST"
		path := "/api/data"
		statusCode := "200"
		host := "memory-test.com"
		clientIP := "192.168.1." + strconv.Itoa(i%10)
		fullURL := path + "?timestamp=" + strconv.Itoa(i)

		b.StartTimer()

		// Record metrics
		uc.requestsTotal.WithLabelValues(statusCode, method, host, path).Inc()
		uc.requestsByIP.WithLabelValues(clientIP, statusCode, method).Inc()
		uc.requestsByURL.WithLabelValues(fullURL, method, statusCode).Inc()
		uc.requestDuration.WithLabelValues(method, statusCode, host).Observe(0.1)
	}
}
