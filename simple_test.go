//go:build !integration
// +build !integration

package caddyusage

import (
	"context"
	"net/http"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
)

// TestGetClientIPSimple tests the client IP extraction logic
// Category: Unit Tests - Basic functionality
func TestGetClientIPSimple(t *testing.T) {
	tests := []struct {
		name         string
		remoteAddr   string
		forwardedFor string
		realIP       string
		expectedIP   string
	}{
		{
			name:         "X-Forwarded-For single IP",
			remoteAddr:   "10.0.0.1:8080",
			forwardedFor: "192.168.1.100",
			expectedIP:   "192.168.1.100",
		},
		{
			name:         "X-Forwarded-For multiple IPs",
			remoteAddr:   "10.0.0.1:8080",
			forwardedFor: "192.168.1.100, 10.0.0.1, 172.16.0.1",
			expectedIP:   "192.168.1.100",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "10.0.0.1:8080",
			realIP:     "203.0.113.1",
			expectedIP: "203.0.113.1",
		},
		{
			name:       "RemoteAddr fallback",
			remoteAddr: "203.0.113.195:45678",
			expectedIP: "203.0.113.195",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				RemoteAddr: tt.remoteAddr,
				Header:     make(http.Header),
			}

			if tt.forwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.forwardedFor)
			}
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}

			result := getClientIP(req)
			if result != tt.expectedIP {
				t.Errorf("Expected IP %s, got %s", tt.expectedIP, result)
			}
		})
	}
}

// TestMetricsRegistration tests that metrics can be registered properly
func TestMetricsRegistration(t *testing.T) {
	uc := setupTestUsageCollector(t)

	verifyMetricsInitialized(t, uc)
	recordTestDataPoints(uc)
	validateRegisteredMetrics(t, uc)
}

func setupTestUsageCollector(t *testing.T) *UsageCollector {
	registry := prometheus.NewRegistry()
	uc := &UsageCollector{
		logger:   zap.NewNop(),
		registry: registry,
	}

	err := uc.registerMetrics()
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	return uc
}

func verifyMetricsInitialized(t *testing.T, uc *UsageCollector) {
	if uc.requestsTotal == nil {
		t.Error("requestsTotal metric not initialized")
	}
	if uc.requestsByIP == nil {
		t.Error("requestsByIP metric not initialized")
	}
	if uc.requestsByURL == nil {
		t.Error("requestsByURL metric not initialized")
	}
	if uc.requestsByHeaders == nil {
		t.Error("requestsByHeaders metric not initialized")
	}
	if uc.requestDuration == nil {
		t.Error("requestDuration metric not initialized")
	}
}

func recordTestDataPoints(uc *UsageCollector) {
	uc.requestsTotal.WithLabelValues("200", "GET", "example.com", "/test").Inc()
	uc.requestsByIP.WithLabelValues("192.168.1.1", "200", "GET").Inc()
	uc.requestsByURL.WithLabelValues("/test?param=value", "GET", "200").Inc()
	uc.requestsByHeaders.WithLabelValues("User-Agent", "test-agent", "GET", "200").Inc()
	uc.requestDuration.WithLabelValues("GET", "200", "example.com").Observe(0.1)
}

func validateRegisteredMetrics(t *testing.T, uc *UsageCollector) {
	metricFamilies, err := uc.registry.(*prometheus.Registry).Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Error("No metrics were registered")
	}

	metricNames := extractMetricNames(metricFamilies)
	checkExpectedMetricsPresent(t, metricNames)
}

func extractMetricNames(metricFamilies []*dto.MetricFamily) map[string]bool {
	metricNames := make(map[string]bool)
	for _, mf := range metricFamilies {
		metricNames[*mf.Name] = true
	}
	return metricNames
}

func checkExpectedMetricsPresent(t *testing.T, metricNames map[string]bool) {
	expectedMetrics := []string{
		"caddy_usage_requests_total",
		"caddy_usage_requests_by_ip_total",
		"caddy_usage_requests_by_url_total",
		"caddy_usage_requests_by_headers_total",
		"caddy_usage_request_duration_seconds",
	}

	for _, expected := range expectedMetrics {
		if !metricNames[expected] {
			t.Errorf("Expected metric %s not found", expected)
		}
	}
}

// TestDuplicateRegistration tests duplicate registration handling
func TestDuplicateRegistration(t *testing.T) {
	// Use a shared registry for this test
	sharedRegistry := prometheus.NewRegistry()

	// Create first instance
	uc1 := &UsageCollector{
		logger:   zap.NewNop(),
		registry: sharedRegistry,
	}

	err := uc1.registerMetrics()
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Create second instance with same registry
	uc2 := &UsageCollector{
		logger:   zap.NewNop(),
		registry: sharedRegistry,
	}

	// This should not fail due to duplicate registration handling
	err = uc2.registerMetrics()
	if err != nil {
		t.Fatalf("Second registration should not fail: %v", err)
	}
}

// TestProvision tests the Provision method
func TestProvision(t *testing.T) {
	uc := &UsageCollector{}
	ctx := caddy.Context{
		Context: context.Background(),
	}

	err := uc.Provision(ctx)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Verify everything was set up
	if uc.logger == nil {
		t.Error("Logger not set after provision")
	}
	if uc.registry == nil {
		t.Error("Registry not set after provision")
	}
	if uc.requestsTotal == nil {
		t.Error("requestsTotal metric not initialized")
	}
}

// TestCaddyModule tests the module info
func TestCaddyModule(t *testing.T) {
	uc := UsageCollector{}
	info := uc.CaddyModule()

	if info.ID != "http.handlers.usage" {
		t.Errorf("Expected module ID 'http.handlers.usage', got '%s'", info.ID)
	}

	if info.New == nil {
		t.Error("New function should not be nil")
	}

	// Test that New() returns correct type
	instance := info.New()
	if _, ok := instance.(*UsageCollector); !ok {
		t.Error("New() should return a *UsageCollector instance")
	}
}

// TestValidate tests the validation
func TestValidate(t *testing.T) {
	uc := &UsageCollector{}
	err := uc.Validate()
	if err != nil {
		t.Errorf("Validate should not return error, got: %v", err)
	}
}
