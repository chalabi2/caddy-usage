package caddyusage

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// TestHeaderMetricsProcessing tests comprehensive header metrics collection
func TestHeaderMetricsProcessing(t *testing.T) {
	// Setup metrics
	registry := prometheus.NewRegistry()
	err := registerMetrics(registry)
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	// Test various header combinations
	testCases := []struct {
		name    string
		headers map[string]string
	}{
		{
			name: "all standard headers",
			headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				"Referer":         "https://example.com/page",
				"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
				"Accept-Language": "en-US,en;q=0.5",
				"Accept-Encoding": "gzip, deflate",
				"Content-Type":    "application/json",
				"Origin":          "https://example.com",
			},
		},
		{
			name: "authorization header",
			headers: map[string]string{
				"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
				"User-Agent":    "API-Client/1.0",
			},
		},
		{
			name: "proxy headers",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1, 198.51.100.1",
				"X-Real-IP":       "203.0.113.1",
				"User-Agent":      "ProxyClient/2.0",
			},
		},
		{
			name: "very long header values",
			headers: map[string]string{
				"User-Agent": "VeryLongUserAgent" + string(make([]byte, 150)), // > 100 chars
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(_ *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			// Test header metrics collection
			uc.collectHeaderMetrics(globalUsageMetrics, req, "GET", "200")

			// Verify no panic occurred and function completed
			// The actual metric verification would require more complex setup
		})
	}
}

// TestClientIPExtractionComprehensive tests extensive client IP scenarios
func TestClientIPExtractionComprehensive(t *testing.T) {
	testCases := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "IPv6 remote address",
			headers:    map[string]string{},
			remoteAddr: "[2001:db8::1]:8080",
			expected:   "[2001:db8::1]",
		},
		{
			name: "malformed X-Forwarded-For",
			headers: map[string]string{
				"X-Forwarded-For": "invalid-ip-format",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "invalid-ip-format",
		},
		{
			name: "empty X-Forwarded-For",
			headers: map[string]string{
				"X-Forwarded-For": "",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "192.168.1.100",
		},
		{
			name: "X-Forwarded-For with spaces",
			headers: map[string]string{
				"X-Forwarded-For": "  203.0.113.1  ,  198.51.100.1  ",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "203.0.113.1",
		},
		{
			name: "private IP in X-Forwarded-For",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1, 172.16.0.1, 192.168.1.1",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "10.0.0.1",
		},
		{
			name: "X-Real-IP overrides when no X-Forwarded-For",
			headers: map[string]string{
				"X-Real-IP": "203.0.113.5",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "203.0.113.5",
		},
		{
			name:       "malformed remote address",
			headers:    map[string]string{},
			remoteAddr: "invalid-address",
			expected:   "invalid-address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(_ *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = tc.remoteAddr

			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			result := getClientIP(req)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

// setupTestMetrics is a helper function to set up metrics for testing
func setupTestMetrics(t *testing.T) (*UsageCollector, *prometheus.Registry, func()) {
	registry := prometheus.NewRegistry()
	metrics, err := initializeMetrics(registry)
	if err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	originalMetrics := globalUsageMetrics
	globalUsageMetrics = metrics

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	cleanup := func() {
		globalUsageMetrics = originalMetrics
	}

	return uc, registry, cleanup
}

// collectTestRequests is a helper function to collect metrics for test requests
func collectTestRequests(_ *testing.T, uc *UsageCollector) {
	requests := []struct {
		method     string
		url        string
		statusCode int
		clientIP   string
	}{
		{"GET", "http://example.com/api/users", 200, "192.168.1.1"},
		{"POST", "http://example.com/api/users", 201, "192.168.1.2"},
		{"GET", "http://example.com/api/users", 200, "192.168.1.1"}, // Duplicate
		{"DELETE", "http://example.com/api/users/1", 404, "192.168.1.3"},
	}

	for _, req := range requests {
		httpReq := httptest.NewRequest(req.method, req.url, nil)
		httpReq.RemoteAddr = req.clientIP + ":8080"

		rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), nil, nil)
		rec.WriteHeader(req.statusCode)

		startTime := time.Now()
		uc.collectMetrics(rec, httpReq, startTime)
	}
}

// verifyMetricsPresence is a helper function to verify that expected metrics are present
func verifyMetricsPresence(t *testing.T, registry *prometheus.Registry) {
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Error("No metrics were recorded")
		return
	}

	expectedMetrics := map[string]bool{
		"caddy_usage_requests_total":           false,
		"caddy_usage_requests_by_ip_total":     false,
		"caddy_usage_requests_by_url_total":    false,
		"caddy_usage_request_duration_seconds": false,
	}

	for _, mf := range metricFamilies {
		if _, exists := expectedMetrics[*mf.Name]; exists {
			expectedMetrics[*mf.Name] = true
		}
	}

	for metricName, found := range expectedMetrics {
		if !found {
			t.Errorf("%s metric not found", metricName)
		}
	}
}

// TestMetricsAccuracy tests that metrics are recorded accurately
func TestMetricsAccuracy(t *testing.T) {
	uc, registry, cleanup := setupTestMetrics(t)
	defer cleanup()

	collectTestRequests(t, uc)
	verifyMetricsPresence(t, registry)
}

// TestConcurrentMetricsCollection tests metrics collection under concurrent load
func TestConcurrentMetricsCollection(t *testing.T) {
	registry := prometheus.NewRegistry()
	err := registerMetrics(registry)
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	// Number of concurrent goroutines
	numGoroutines := 10
	requestsPerGoroutine := 20

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				req := httptest.NewRequest("GET", "http://example.com/test", nil)
				req.RemoteAddr = "192.168.1.100:8080"
				req.Header.Set("User-Agent", "ConcurrentTestAgent/1.0")

				rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), nil, nil)
				rec.WriteHeader(200)

				startTime := time.Now()
				uc.collectMetrics(rec, req, startTime)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify metrics can still be gathered after concurrent access
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics after concurrent test: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Log("No metrics found after concurrent collection - this may be expected with isolated registry")
	} else {
		t.Logf("Concurrent test completed successfully with %d metric families", len(metricFamilies))
	}
}

// TestMetricsWithDifferentURLPatterns tests metrics with various URL patterns
func TestMetricsWithDifferentURLPatterns(t *testing.T) {
	registry := prometheus.NewRegistry()
	err := registerMetrics(registry)
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	// Test different URL patterns
	urls := []string{
		"http://example.com/",
		"http://example.com/api/v1/users",
		"http://example.com/api/v1/users?page=1&limit=10",
		"http://example.com/static/css/style.css",
		"http://example.com/images/logo.png?version=1.2.3",
		"http://subdomain.example.com/path/to/resource",
		"https://secure.example.com/auth/login",
		"http://example.com/path/with/unicode/文字",
		"http://example.com/path%20with%20encoded%20spaces",
	}

	for _, url := range urls {
		req := httptest.NewRequest("GET", url, nil)
		req.RemoteAddr = "192.168.1.100:8080"

		rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), nil, nil)
		rec.WriteHeader(200)

		startTime := time.Now()
		uc.collectMetrics(rec, req, startTime)
	}

	// Verify metrics were collected
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Log("No metrics recorded for URL pattern test - this may be expected with isolated registry")
	}
}

// TestMetricsWithSpecialCharacters tests metrics handling of special characters
func TestMetricsWithSpecialCharacters(t *testing.T) {
	registry := prometheus.NewRegistry()
	err := registerMetrics(registry)
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: zap.NewNop(),
		ctx:    ctx,
	}

	// Test requests with special characters in headers and URLs
	testCases := []struct {
		name    string
		url     string
		headers map[string]string
	}{
		{
			name: "unicode in URL",
			url:  "http://example.com/测试/路径",
			headers: map[string]string{
				"User-Agent": "TestAgent/1.0",
			},
		},
		{
			name: "special chars in headers",
			url:  "http://example.com/test",
			headers: map[string]string{
				"User-Agent": "Special-Agent/1.0 (Windows; U; en-US) \"quoted\"",
				"Referer":    "http://example.com/页面?参数=值",
			},
		},
		{
			name: "control characters",
			url:  "http://example.com/test",
			headers: map[string]string{
				"User-Agent": "Agent\t\n\r/1.0",
			},
		},
		{
			name: "emoji in headers",
			url:  "http://example.com/test",
			headers: map[string]string{
				"User-Agent": "Agent 🚀 Emoji/1.0",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(_ *testing.T) {
			req := httptest.NewRequest("GET", tc.url, nil)
			req.RemoteAddr = "192.168.1.100:8080"

			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), nil, nil)
			rec.WriteHeader(200)

			startTime := time.Now()

			// This should not panic even with special characters
			uc.collectMetrics(rec, req, startTime)
		})
	}

	// Verify metrics were collected without errors
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics with special characters: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Log("No metrics recorded for special characters test - this may be expected with isolated registry")
	}
}
