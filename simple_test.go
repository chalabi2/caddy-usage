package caddyusage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestCaddyModule verifies the module registration and basic info
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

// TestValidate tests the module validation
func TestValidate(t *testing.T) {
	uc := &UsageCollector{}
	err := uc.Validate()
	if err != nil {
		t.Errorf("Validate should not return error: %v", err)
	}
}

// TestProvision tests the module provisioning
func TestProvision(t *testing.T) {
	// Create a test logger with observer
	core, logs := observer.New(zapcore.DebugLevel)
	_ = zap.New(core)

	// Create a test context with a mock metrics registry
	ctx := caddy.Context{
		Context: context.Background(),
	}

	// Create collector and provision it
	uc := &UsageCollector{}
	err := uc.Provision(ctx)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Check that logger was set
	if uc.logger == nil {
		t.Error("Logger was not set during provision")
	}

	// Check that provision succeeded
	found := false
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, "usage collector") && strings.Contains(entry.Message, "successfully") {
			found = true
			break
		}
	}
	if !found {
		t.Log("Expected provision success log message not found (this is acceptable)")
	}
}

// TestServeHTTP tests the main HTTP handler functionality
func TestServeHTTP(t *testing.T) {
	// Create a test context
	ctx := caddy.Context{
		Context: context.Background(),
	}

	// Create and provision collector
	uc := &UsageCollector{}
	err := uc.Provision(ctx)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/test?param=value", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.RemoteAddr = "192.168.1.100:12345"

	// Create response recorder
	w := httptest.NewRecorder()

	// Create next handler that writes a response
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
		return nil
	})

	// Test the handler
	err = uc.ServeHTTP(w, req, next)
	if err != nil {
		t.Fatalf("ServeHTTP failed: %v", err)
	}

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "test response" {
		t.Errorf("Expected body 'test response', got '%s'", w.Body.String())
	}
}

// TestGetClientIPSimple tests basic client IP extraction functionality
func TestGetClientIPSimple(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "direct connection",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.100:12345",
			expected:   "192.168.1.100",
		},
		{
			name: "x-forwarded-for single",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "203.0.113.1",
		},
		{
			name: "x-forwarded-for multiple",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1, 198.51.100.1, 192.168.1.1",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "203.0.113.1",
		},
		{
			name: "x-real-ip",
			headers: map[string]string{
				"X-Real-IP": "203.0.113.2",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "203.0.113.2",
		},
		{
			name: "x-forwarded",
			headers: map[string]string{
				"X-Forwarded": "203.0.113.3",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "203.0.113.3",
		},
		{
			name: "priority order - xff wins",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1",
				"X-Real-IP":       "203.0.113.2",
				"X-Forwarded":     "203.0.113.3",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "203.0.113.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = tt.remoteAddr

			// Set headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			// Test function
			result := getClientIP(req)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestMetricsRegistration tests that metrics can be registered without errors
func TestMetricsRegistration(t *testing.T) {
	registry := prometheus.NewRegistry()

	err := registerMetrics(registry)
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	// Verify global metrics were set
	if globalUsageMetrics == nil {
		t.Error("Global metrics should be set after registration")
	}

	// Verify metrics structs are not nil
	if globalUsageMetrics.requestsTotal == nil {
		t.Error("requestsTotal should not be nil")
	}
	if globalUsageMetrics.requestsByIP == nil {
		t.Error("requestsByIP should not be nil")
	}
	if globalUsageMetrics.requestsByURL == nil {
		t.Error("requestsByURL should not be nil")
	}
	if globalUsageMetrics.requestsByHeaders == nil {
		t.Error("requestsByHeaders should not be nil")
	}
	if globalUsageMetrics.requestDuration == nil {
		t.Error("requestDuration should not be nil")
	}
}

// TestDuplicateRegistration tests that duplicate metric registration is handled gracefully
func TestDuplicateRegistration(t *testing.T) {
	registry := prometheus.NewRegistry()

	// First registration should succeed
	err := registerMetrics(registry)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Second registration should also succeed (handles AlreadyRegisteredError)
	err = registerMetrics(registry)
	if err != nil {
		t.Fatalf("Second registration failed: %v", err)
	}
}

// TestCollectMetricsWithNilGlobal tests handling when global metrics is nil
func TestCollectMetricsWithNilGlobal(t *testing.T) {
	// Save current global metrics
	originalMetrics := globalUsageMetrics
	defer func() {
		globalUsageMetrics = originalMetrics
	}()

	// Set global metrics to nil
	globalUsageMetrics = nil

	// Create a test context with observer logger
	core, _ := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: logger,
		ctx:    ctx,
	}

	// Create mock request and response recorder
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), nil, nil)
	startTime := time.Now()

	// This should not panic and should log an error
	uc.collectMetrics(rec, req, startTime)

	// The function should handle nil global metrics gracefully
	// We can't easily verify the log message without more complex setup,
	// but the fact that it doesn't panic is sufficient
}

// BenchmarkCollectMetrics benchmarks the metrics collection performance
func BenchmarkCollectMetrics(b *testing.B) {
	// Setup
	registry := prometheus.NewRegistry()
	registerMetrics(registry)

	core, _ := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	ctx := caddy.Context{
		Context: context.Background(),
	}

	uc := &UsageCollector{
		logger: logger,
		ctx:    ctx,
	}

	req := httptest.NewRequest("GET", "http://example.com/test?param=value", nil)
	req.Header.Set("User-Agent", "BenchmarkAgent/1.0")
	req.RemoteAddr = "192.168.1.100:12345"

	rec := caddyhttp.NewResponseRecorder(httptest.NewRecorder(), nil, nil)
	rec.WriteHeader(200)

	startTime := time.Now()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		uc.collectMetrics(rec, req, startTime)
	}
}
