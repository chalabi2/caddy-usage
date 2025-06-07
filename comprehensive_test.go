//go:build !integration
// +build !integration

package caddyusage

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
)

// TestHeaderMetricsProcessing tests header processing logic
// Category: Comprehensive Tests - Advanced functionality
func TestHeaderMetricsProcessing(t *testing.T) {
	testCases := getHeaderTestCases()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uc := setupUsageCollectorForTest(t)
			req := createRequestWithHeader(tc.headerName, tc.headerValue)

			uc.collectHeaderMetrics(req, "GET", "200")

			validateHeaderMetrics(t, uc, tc)
		})
	}
}

type headerTestCase struct {
	name            string
	headerName      string
	headerValue     string
	expectedValue   string
	shouldBeTracked bool
}

func getHeaderTestCases() []headerTestCase {
	return []headerTestCase{
		{
			name:            "Normal User-Agent",
			headerName:      "User-Agent",
			headerValue:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
			expectedValue:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
			shouldBeTracked: true,
		},
		{
			name:            "Authorization header should be masked",
			headerName:      "Authorization",
			headerValue:     "Bearer eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9",
			expectedValue:   "present",
			shouldBeTracked: true,
		},
		{
			name:            "Very long header should be truncated",
			headerName:      "Accept",
			headerValue:     strings.Repeat("a", 150),
			expectedValue:   strings.Repeat("a", 100) + "...",
			shouldBeTracked: true,
		},
		{
			name:            "Empty header should not be tracked",
			headerName:      "User-Agent",
			headerValue:     "",
			shouldBeTracked: false,
		},
		{
			name:            "Untracked header should not be processed",
			headerName:      "X-Custom-Header",
			headerValue:     "custom-value",
			shouldBeTracked: false,
		},
	}
}

func setupUsageCollectorForTest(t *testing.T) *UsageCollector {
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

func createRequestWithHeader(headerName, headerValue string) *http.Request {
	req := &http.Request{
		Header: make(http.Header),
	}

	if headerValue != "" {
		req.Header.Set(headerName, headerValue)
	}

	return req
}

func validateHeaderMetrics(t *testing.T, uc *UsageCollector, tc headerTestCase) {
	metricFamilies, err := uc.registry.(*prometheus.Registry).Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := findHeaderMetric(metricFamilies, tc.headerName, tc.expectedValue)

	if tc.shouldBeTracked && !found {
		t.Errorf("Expected header %s to be tracked but was not found", tc.headerName)
	} else if !tc.shouldBeTracked && found {
		t.Errorf("Expected header %s not to be tracked but was found", tc.headerName)
	}
}

func findHeaderMetric(metricFamilies []*dto.MetricFamily, expectedHeaderName, expectedHeaderValue string) bool {
	for _, mf := range metricFamilies {
		if *mf.Name == "caddy_usage_requests_by_headers_total" {
			for _, metric := range mf.Metric {
				headerName, headerValue := extractHeaderLabels(metric.GetLabel())
				if headerName == expectedHeaderName {
					if expectedHeaderValue != "" && headerValue != expectedHeaderValue {
						return false
					}
					return true
				}
			}
		}
	}
	return false
}

func extractHeaderLabels(labels []*dto.LabelPair) (string, string) {
	var headerName, headerValue string
	for _, label := range labels {
		switch *label.Name {
		case "header_name":
			headerName = *label.Value
		case "header_value":
			headerValue = *label.Value
		}
	}
	return headerName, headerValue
}

// TestClientIPExtractionComprehensive tests all IP extraction scenarios
func TestClientIPExtractionComprehensive(t *testing.T) {
	testCases := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		expectedIP string
	}{
		{
			name:       "X-Forwarded-For single IP",
			remoteAddr: "10.0.0.1:8080",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1",
			},
			expectedIP: "203.0.113.1",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			remoteAddr: "10.0.0.1:8080",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1, 198.51.100.1, 10.0.0.1",
			},
			expectedIP: "203.0.113.1",
		},
		{
			name:       "X-Forwarded-For with spaces",
			remoteAddr: "10.0.0.1:8080",
			headers: map[string]string{
				"X-Forwarded-For": "  203.0.113.5  , 198.51.100.1",
			},
			expectedIP: "203.0.113.5",
		},
		{
			name:       "X-Real-IP takes precedence when X-Forwarded-For is empty",
			remoteAddr: "10.0.0.1:8080",
			headers: map[string]string{
				"X-Real-IP": "203.0.113.2",
			},
			expectedIP: "203.0.113.2",
		},
		{
			name:       "X-Forwarded header fallback",
			remoteAddr: "10.0.0.1:8080",
			headers: map[string]string{
				"X-Forwarded": "203.0.113.3",
			},
			expectedIP: "203.0.113.3",
		},
		{
			name:       "RemoteAddr fallback",
			remoteAddr: "203.0.113.4:12345",
			headers:    map[string]string{},
			expectedIP: "203.0.113.4",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{
				RemoteAddr: tc.remoteAddr,
				Header:     make(http.Header),
			}

			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			result := getClientIP(req)
			if result != tc.expectedIP {
				t.Errorf("Expected IP %s, got %s", tc.expectedIP, result)
			}
		})
	}
}

// TestMetricsAccuracy tests that metrics are recorded with correct values
func TestMetricsAccuracy(t *testing.T) {
	uc := setupUsageCollectorForTest(t)
	testData := getMetricsTestData()

	recordTestMetrics(uc, testData)
	validateMetricsAccuracy(t, uc, testData)
}

type metricsTestData struct {
	method     string
	path       string
	statusCode string
	host       string
	count      int
}

func getMetricsTestData() []metricsTestData {
	return []metricsTestData{
		{"GET", "/api/users", "200", "test.com", 3},
		{"POST", "/api/users", "201", "test.com", 2},
		{"GET", "/api/users", "404", "test.com", 1},
		{"DELETE", "/api/users/1", "204", "test.com", 1},
	}
}

func recordTestMetrics(uc *UsageCollector, testData []metricsTestData) {
	for _, data := range testData {
		for i := 0; i < data.count; i++ {
			uc.requestsTotal.WithLabelValues(data.statusCode, data.method, data.host, data.path).Inc()
		}
	}
}

func validateMetricsAccuracy(t *testing.T, uc *UsageCollector, testData []metricsTestData) {
	metricFamilies, err := uc.registry.(*prometheus.Registry).Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	for _, mf := range metricFamilies {
		if *mf.Name == "caddy_usage_requests_total" {
			validateRequestTotalMetrics(t, mf.Metric, testData)
		}
	}
}

func validateRequestTotalMetrics(t *testing.T, metrics []*dto.Metric, testData []metricsTestData) {
	for _, metric := range metrics {
		method, path, statusCode := extractRequestLabels(metric.GetLabel())
		expectedCount := findExpectedCount(testData, method, path, statusCode)
		actualCount := int(metric.Counter.GetValue())

		if actualCount != expectedCount {
			t.Errorf("For %s %s %s: expected count %d, got %d",
				method, path, statusCode, expectedCount, actualCount)
		}
	}
}

func extractRequestLabels(labels []*dto.LabelPair) (string, string, string) {
	var method, path, statusCode string
	for _, label := range labels {
		switch *label.Name {
		case "method":
			method = *label.Value
		case "path":
			path = *label.Value
		case "status_code":
			statusCode = *label.Value
		}
	}
	return method, path, statusCode
}

func findExpectedCount(testData []metricsTestData, method, path, statusCode string) int {
	for _, data := range testData {
		if data.method == method && data.path == path && data.statusCode == statusCode {
			return data.count
		}
	}
	return 0
}

// TestConcurrentMetricsCollection tests thread safety
func TestConcurrentMetricsCollection(t *testing.T) {
	registry := prometheus.NewRegistry()
	uc := &UsageCollector{
		logger:   zap.NewNop(),
		registry: registry,
	}

	err := uc.registerMetrics()
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	// Number of concurrent goroutines
	numGoroutines := 10
	requestsPerGoroutine := 50
	totalExpected := numGoroutines * requestsPerGoroutine

	done := make(chan bool, numGoroutines)

	// Launch concurrent metric recording
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < requestsPerGoroutine; j++ {
				uc.requestsTotal.WithLabelValues("200", "GET", "concurrent.test", "/test").Inc()
				uc.requestsByIP.WithLabelValues("192.168.1."+strconv.Itoa(id), "200", "GET").Inc()
				uc.requestDuration.WithLabelValues("GET", "200", "concurrent.test").Observe(0.1)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify total count
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	for _, mf := range metricFamilies {
		if *mf.Name == "caddy_usage_requests_total" {
			if len(mf.Metric) > 0 {
				totalCount := int(mf.Metric[0].Counter.GetValue())
				if totalCount != totalExpected {
					t.Errorf("Expected total count %d, got %d", totalExpected, totalCount)
				}
			}
		}
	}
}

// TestProvisionWithDifferentContexts tests Provision with various contexts
func TestProvisionWithDifferentContexts(t *testing.T) {
	testCases := []struct {
		name        string
		setupCtx    func() caddy.Context
		expectError bool
	}{
		{
			name: "Valid context",
			setupCtx: func() caddy.Context {
				return caddy.Context{
					Context: context.Background(),
				}
			},
			expectError: false,
		},
		{
			name: "Context with timeout",
			setupCtx: func() caddy.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return caddy.Context{
					Context: ctx,
				}
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &UsageCollector{}
			ctx := tc.setupCtx()

			err := uc.Provision(ctx)

			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			} else if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tc.expectError {
				// Verify provision set up everything correctly
				if uc.logger == nil {
					t.Error("Logger not set after provision")
				}
				if uc.registry == nil {
					t.Error("Registry not set after provision")
				}
				if uc.requestsTotal == nil {
					t.Error("Metrics not initialized after provision")
				}
			}
		})
	}
}

// TestMetricsWithDifferentURLPatterns tests URL pattern handling
func TestMetricsWithDifferentURLPatterns(t *testing.T) {
	uc := setupUsageCollectorForTest(t)
	urlPatterns := getTestURLPatterns()

	recordURLMetrics(uc, urlPatterns)
	validateURLPatternMetrics(t, uc, urlPatterns)
}

func getTestURLPatterns() []string {
	return []string{
		"/api/users",
		"/api/users/123",
		"/api/posts?page=1&limit=10",
		"/static/css/style.css",
		"/uploads/images/photo.jpg",
		"/api/search?q=golang&type=code&sort=updated",
		"/very/long/path/with/many/segments/that/tests/url/handling",
	}
}

func recordURLMetrics(uc *UsageCollector, urlPatterns []string) {
	for _, url := range urlPatterns {
		uc.requestsByURL.WithLabelValues(url, "GET", "200").Inc()
	}
}

func validateURLPatternMetrics(t *testing.T, uc *UsageCollector, expectedURLs []string) {
	metricFamilies, err := uc.registry.(*prometheus.Registry).Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	foundURLs := extractURLsFromMetrics(metricFamilies)
	checkURLPatternsPresent(t, foundURLs, expectedURLs)
}

func extractURLsFromMetrics(metricFamilies []*dto.MetricFamily) map[string]bool {
	foundURLs := make(map[string]bool)
	for _, mf := range metricFamilies {
		if *mf.Name == "caddy_usage_requests_by_url_total" {
			for _, metric := range mf.Metric {
				labels := metric.GetLabel()
				for _, label := range labels {
					if *label.Name == "full_url" {
						foundURLs[*label.Value] = true
					}
				}
			}
		}
	}
	return foundURLs
}

func checkURLPatternsPresent(t *testing.T, foundURLs map[string]bool, expectedURLs []string) {
	for _, expectedURL := range expectedURLs {
		if !foundURLs[expectedURL] {
			t.Errorf("URL pattern %s not found in metrics", expectedURL)
		}
	}

	if len(foundURLs) != len(expectedURLs) {
		t.Errorf("Expected %d URL patterns, found %d", len(expectedURLs), len(foundURLs))
	}
}

// TestMetricsWithSpecialCharacters tests handling of special characters
func TestMetricsWithSpecialCharacters(t *testing.T) {
	registry := prometheus.NewRegistry()
	uc := &UsageCollector{
		logger:   zap.NewNop(),
		registry: registry,
	}

	err := uc.registerMetrics()
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	specialCases := []struct {
		name        string
		headerValue string
		description string
	}{
		{
			name:        "Unicode characters",
			headerValue: "Mozilla/5.0 (测试浏览器)",
			description: "Unicode in User-Agent",
		},
		{
			name:        "Special symbols",
			headerValue: "application/json; charset=utf-8",
			description: "Content-Type with special chars",
		},
		{
			name:        "URL encoded",
			headerValue: "search%20query%20with%20spaces",
			description: "URL encoded content",
		},
	}

	for _, tc := range specialCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{
				Header: make(http.Header),
			}
			req.Header.Set("User-Agent", tc.headerValue)

			// This should not panic or cause errors
			uc.collectHeaderMetrics(req, "GET", "200")

			// Verify metrics were recorded
			metricFamilies, err := registry.Gather()
			if err != nil {
				t.Fatalf("Failed to gather metrics: %v", err)
			}

			// Just verify we can gather metrics without errors
			if len(metricFamilies) == 0 {
				t.Error("No metrics found after processing special characters")
			}
		})
	}
}

// TestValidateMethod tests the Validate method
func TestValidateMethod(t *testing.T) {
	uc := &UsageCollector{}

	err := uc.Validate()
	if err != nil {
		t.Errorf("Validate should not return error, got: %v", err)
	}
}

// TestModuleInfo tests the CaddyModule method
func TestModuleInfo(t *testing.T) {
	uc := UsageCollector{}
	info := uc.CaddyModule()

	expectedID := "http.handlers.usage"
	if string(info.ID) != expectedID {
		t.Errorf("Expected module ID %s, got %s", expectedID, string(info.ID))
	}

	if info.New == nil {
		t.Error("New function should not be nil")
	}

	// Test that New() returns a new instance
	newInstance := info.New()
	if newInstance == nil {
		t.Error("New() should return a non-nil instance")
	}

	if _, ok := newInstance.(*UsageCollector); !ok {
		t.Error("New() should return a *UsageCollector instance")
	}
}
