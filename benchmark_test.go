package caddyusage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// BenchmarkClientIPExtraction benchmarks the client IP extraction performance
func BenchmarkClientIPExtraction(b *testing.B) {
	// Setup test requests with different scenarios
	testCases := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
	}{
		{
			name:       "direct_connection",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.100:12345",
		},
		{
			name: "x_forwarded_for_single",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1",
			},
			remoteAddr: "192.168.1.100:12345",
		},
		{
			name: "x_forwarded_for_multiple",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1, 198.51.100.1, 192.168.1.1",
			},
			remoteAddr: "192.168.1.100:12345",
		},
		{
			name: "x_real_ip",
			headers: map[string]string{
				"X-Real-IP": "203.0.113.2",
			},
			remoteAddr: "192.168.1.100:12345",
		},
		{
			name: "all_headers",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1, 198.51.100.1",
				"X-Real-IP":       "203.0.113.2",
				"X-Forwarded":     "203.0.113.3",
			},
			remoteAddr: "192.168.1.100:12345",
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Pre-create request to avoid allocation overhead in benchmark
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = tc.remoteAddr
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = getClientIP(req)
			}
		})
	}
}

// BenchmarkHeaderMetricsCollection benchmarks header metrics collection performance
func BenchmarkHeaderMetricsCollection(b *testing.B) {
	// Setup metrics
	registry := prometheus.NewRegistry()
	err := registerMetrics(registry)
	if err != nil {
		b.Fatalf("Failed to register metrics: %v", err)
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	// Test different header scenarios
	testCases := []struct {
		name    string
		headers map[string]string
	}{
		{
			name: "minimal_headers",
			headers: map[string]string{
				"User-Agent": "TestAgent/1.0",
			},
		},
		{
			name: "standard_headers",
			headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
				"Accept-Language": "en-US,en;q=0.5",
				"Accept-Encoding": "gzip, deflate",
			},
		},
		{
			name: "comprehensive_headers",
			headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				"Referer":         "https://example.com/page",
				"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
				"Accept-Language": "en-US,en;q=0.5",
				"Accept-Encoding": "gzip, deflate",
				"Content-Type":    "application/json",
				"Authorization":   "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
				"X-Forwarded-For": "203.0.113.1, 198.51.100.1",
				"X-Real-IP":       "203.0.113.1",
				"Origin":          "https://example.com",
			},
		},
		{
			name: "long_headers",
			headers: map[string]string{
				"User-Agent": "VeryLongUserAgent" + string(make([]byte, 200)), // Long header value
				"Referer":    "https://example.com/very/long/path/with/many/segments/that/could/be/typical/in/real/applications",
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Pre-create request to avoid allocation overhead
			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				uc.collectHeaderMetrics(globalUsageMetrics, req, "GET", "200")
			}
		})
	}
}

// BenchmarkMetricsRegistration benchmarks metrics registration performance
func BenchmarkMetricsRegistration(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create a new registry for each iteration to benchmark fresh registration
		registry := prometheus.NewRegistry()

		err := registerMetrics(registry)
		if err != nil {
			b.Fatalf("Failed to register metrics: %v", err)
		}
	}
}

// BenchmarkCompleteRequestFlow benchmarks the complete request processing flow
func BenchmarkCompleteRequestFlow(b *testing.B) {
	// Setup
	registry := prometheus.NewRegistry()
	err := registerMetrics(registry)
	if err != nil {
		b.Fatalf("Failed to register metrics: %v", err)
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	// Create next handler
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
		return nil
	})

	// Test different request scenarios
	testCases := []struct {
		name    string
		method  string
		url     string
		headers map[string]string
	}{
		{
			name:   "simple_get",
			method: "GET",
			url:    "http://example.com/",
			headers: map[string]string{
				"User-Agent": "TestAgent/1.0",
			},
		},
		{
			name:   "api_post",
			method: "POST",
			url:    "http://example.com/api/users",
			headers: map[string]string{
				"User-Agent":    "APIClient/1.0",
				"Content-Type":  "application/json",
				"Authorization": "Bearer token123",
			},
		},
		{
			name:   "complex_request",
			method: "PUT",
			url:    "http://example.com/api/users/123?include=profile&fields=name,email",
			headers: map[string]string{
				"User-Agent":      "ComplexClient/2.0",
				"Content-Type":    "application/json",
				"Accept":          "application/json",
				"Accept-Language": "en-US,en;q=0.9",
				"Authorization":   "Bearer complextoken",
				"X-Forwarded-For": "203.0.113.1, 198.51.100.1",
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Create request
				req := httptest.NewRequest(tc.method, tc.url, nil)
				req.RemoteAddr = "192.168.1.100:8080"
				for key, value := range tc.headers {
					req.Header.Set(key, value)
				}

				// Create response recorder
				w := httptest.NewRecorder()

				// Process through middleware
				err := uc.ServeHTTP(w, req, next)
				if err != nil {
					b.Fatalf("ServeHTTP failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkMetricsCollection benchmarks just the metrics collection part
func BenchmarkMetricsCollection(b *testing.B) {
	// Setup
	registry := prometheus.NewRegistry()
	err := registerMetrics(registry)
	if err != nil {
		b.Fatalf("Failed to register metrics: %v", err)
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	// Pre-create request and response recorder
	req := httptest.NewRequest("GET", "http://example.com/api/test?param=value", nil)
	req.Header.Set("User-Agent", "BenchmarkAgent/1.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer benchmarktoken")
	req.RemoteAddr = "192.168.1.100:8080"

	rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), nil, nil)
	rec.WriteHeader(200)

	startTime := time.Now()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		uc.collectMetrics(rec, req, startTime)
	}
}

// BenchmarkConcurrentMetricsCollection benchmarks concurrent metrics collection
func BenchmarkConcurrentMetricsCollection(b *testing.B) {
	// Setup
	registry := prometheus.NewRegistry()
	err := registerMetrics(registry)
	if err != nil {
		b.Fatalf("Failed to register metrics: %v", err)
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	// Pre-create request and response recorder
	req := httptest.NewRequest("GET", "http://example.com/concurrent", nil)
	req.Header.Set("User-Agent", "ConcurrentBenchmarkAgent/1.0")
	req.RemoteAddr = "192.168.1.100:8080"

	rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), nil, nil)
	rec.WriteHeader(200)

	startTime := time.Now()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			uc.collectMetrics(rec, req, startTime)
		}
	})
}

// BenchmarkMemoryUsage benchmarks memory allocations during metrics collection
func BenchmarkMemoryUsage(b *testing.B) {
	// Setup
	registry := prometheus.NewRegistry()
	err := registerMetrics(registry)
	if err != nil {
		b.Fatalf("Failed to register metrics: %v", err)
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	// Different request types to test memory usage patterns
	testCases := []struct {
		name string
		req  *http.Request
	}{
		{
			name: "minimal_request",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "http://example.com/", nil)
				r.RemoteAddr = "192.168.1.100:8080"
				return r
			}(),
		},
		{
			name: "complex_request",
			req: func() *http.Request {
				r := httptest.NewRequest("POST", "http://example.com/api/complex/endpoint?param1=value1&param2=value2", nil)
				r.Header.Set("User-Agent", "ComplexAgent/1.0 (Platform; OS Version) Engine/Version")
				r.Header.Set("Accept", "application/json, text/plain, */*")
				r.Header.Set("Content-Type", "application/json; charset=utf-8")
				r.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c")
				r.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18, 150.172.238.178")
				r.RemoteAddr = "192.168.1.100:8080"
				return r
			}(),
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), nil, nil)
			rec.WriteHeader(200)
			startTime := time.Now()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				uc.collectMetrics(rec, tc.req, startTime)
			}
		})
	}
}
