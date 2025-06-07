//go:build integration
// +build integration

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

// TestIntegrationUsageCollectorMiddleware tests the full middleware integration
// Category: Integration Tests - Full middleware functionality
func TestIntegrationUsageCollectorMiddleware(t *testing.T) {
	// Create a new registry for this integration test
	registry := prometheus.NewRegistry()

	// Create and provision the usage collector
	uc := &UsageCollector{
		logger:   zap.NewNop(),
		registry: registry,
	}

	ctx := caddy.Context{
		Context: context.Background(),
	}

	err := uc.Provision(ctx)
	if err != nil {
		t.Fatalf("Failed to provision usage collector: %v", err)
	}

	// Override the registry after provision to use our test registry
	uc.registry = registry

	// Create a mock next handler
	nextHandler := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
		return nil
	})

	// Create test requests
	testCases := []struct {
		name     string
		method   string
		url      string
		headers  map[string]string
		expected int
	}{
		{
			name:     "GET request",
			method:   "GET",
			url:      "/api/users",
			headers:  map[string]string{"User-Agent": "Test-Agent/1.0"},
			expected: http.StatusOK,
		},
		{
			name:     "POST request with auth",
			method:   "POST",
			url:      "/api/users",
			headers:  map[string]string{"Authorization": "Bearer token123"},
			expected: http.StatusOK,
		},
		{
			name:   "Request with complex headers",
			method: "PUT",
			url:    "/api/users/123?include=profile",
			headers: map[string]string{
				"Content-Type":    "application/json",
				"Accept":          "application/json",
				"Accept-Language": "en-US,en;q=0.9",
				"X-Forwarded-For": "203.0.113.1, 198.51.100.1",
			},
			expected: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest(tc.method, tc.url, nil)

			// Add headers
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			// Create response recorder
			w := httptest.NewRecorder()

			// Process request through middleware
			err := uc.ServeHTTP(w, req, nextHandler)
			if err != nil {
				t.Fatalf("Middleware returned error: %v", err)
			}

			// Verify response
			if w.Code != tc.expected {
				t.Errorf("Expected status %d, got %d", tc.expected, w.Code)
			}

			// Small delay to ensure metrics are recorded
			time.Sleep(10 * time.Millisecond)
		})
	}

	// Verify metrics were recorded
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Error("No metrics were recorded during integration test")
	}

	// Verify specific metrics exist
	expectedMetrics := []string{
		"caddy_usage_requests_total",
		"caddy_usage_requests_by_ip_total",
		"caddy_usage_requests_by_url_total",
		"caddy_usage_requests_by_headers_total",
		"caddy_usage_request_duration_seconds",
	}

	foundMetrics := make(map[string]bool)
	for _, mf := range metricFamilies {
		foundMetrics[*mf.Name] = true
	}

	for _, expectedMetric := range expectedMetrics {
		if !foundMetrics[expectedMetric] {
			t.Errorf("Expected metric %s not found in integration test", expectedMetric)
		}
	}

	t.Logf("Integration test completed successfully with %d metric families", len(metricFamilies))
}

// TestIntegrationMetricsEndToEnd tests the complete metrics collection flow
// Category: Integration Tests - End-to-end metrics collection
func TestIntegrationMetricsEndToEnd(t *testing.T) {
	registry := prometheus.NewRegistry()
	uc := &UsageCollector{
		logger:   zap.NewNop(),
		registry: registry,
	}

	ctx := caddy.Context{Context: context.Background()}
	if err := uc.Provision(ctx); err != nil {
		t.Fatalf("Failed to provision: %v", err)
	}

	// Override the registry after provision to use our test registry
	uc.registry = registry

	// Simulate multiple requests
	requests := []struct {
		path   string
		method string
		ip     string
	}{
		{"/api/v1/users", "GET", "192.168.1.100"},
		{"/api/v1/users/1", "PUT", "192.168.1.101"},
		{"/api/v1/posts", "POST", "192.168.1.102"},
		{"/static/css/app.css", "GET", "192.168.1.100"},
	}

	nextHandler := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	for _, req := range requests {
		httpReq := httptest.NewRequest(req.method, req.path, nil)
		httpReq.RemoteAddr = req.ip + ":8080"
		httpReq.Header.Set("User-Agent", "Integration-Test/1.0")

		w := httptest.NewRecorder()

		if err := uc.ServeHTTP(w, httpReq, nextHandler); err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	}

	// Verify metrics contain expected data
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Count total requests recorded
	var totalRequests float64
	for _, mf := range metricFamilies {
		if *mf.Name == "caddy_usage_requests_total" {
			for _, metric := range mf.Metric {
				totalRequests += metric.Counter.GetValue()
			}
		}
	}

	expectedRequests := float64(len(requests))
	if totalRequests != expectedRequests {
		t.Errorf("Expected %v total requests recorded, got %v", expectedRequests, totalRequests)
	}

	t.Logf("End-to-end test recorded %v requests successfully", totalRequests)
}
