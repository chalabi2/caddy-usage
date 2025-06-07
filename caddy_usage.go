package caddyusage

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(UsageCollector{})
	httpcaddyfile.RegisterHandlerDirective("usage", parseCaddyfile)
}

// UsageCollector is a Caddy HTTP handler that collects comprehensive request metrics
// and exports them to Prometheus. It tracks response status codes, client IPs,
// requested URLs, and request headers.
type UsageCollector struct {
	// Logger for debug and error messages
	logger *zap.Logger

	// Prometheus metrics
	requestsTotal     *prometheus.CounterVec
	requestsByIP      *prometheus.CounterVec
	requestsByURL     *prometheus.CounterVec
	requestsByHeaders *prometheus.CounterVec
	requestDuration   *prometheus.HistogramVec

	// Metrics registry for proper cleanup
	registry prometheus.Registerer
}

// CaddyModule returns the Caddy module information
func (UsageCollector) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.usage",
		New: func() caddy.Module { return new(UsageCollector) },
	}
}

// Provision sets up the UsageCollector with necessary resources
func (uc *UsageCollector) Provision(ctx caddy.Context) error {
	uc.logger = ctx.Logger(uc)

	// Use default Prometheus registry
	uc.registry = prometheus.DefaultRegisterer

	// Initialize metrics
	if err := uc.registerMetrics(); err != nil {
		return err
	}

	uc.logger.Info("usage collector provisioned successfully")
	return nil
}

// registerMetrics initializes and registers all Prometheus metrics for request tracking
// This handles duplicate registration errors gracefully for config reloads
func (uc *UsageCollector) registerMetrics() error {
	uc.initializeMetrics()
	return uc.registerAllMetrics()
}

// initializeMetrics creates all the Prometheus metrics
func (uc *UsageCollector) initializeMetrics() {
	// Total requests by status code, method, and host
	uc.requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "caddy_usage_requests_total",
			Help: "Total number of HTTP requests by status code, method, and host",
		},
		[]string{"status_code", "method", "host", "path"},
	)

	// Requests by client IP address
	uc.requestsByIP = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "caddy_usage_requests_by_ip_total",
			Help: "Total number of requests by client IP address",
		},
		[]string{"client_ip", "status_code", "method"},
	)

	// Requests by exact URL path and query parameters
	uc.requestsByURL = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "caddy_usage_requests_by_url_total",
			Help: "Total number of requests by exact URL path and query parameters",
		},
		[]string{"full_url", "method", "status_code"},
	)

	// Requests by specific headers (User-Agent, Referer, etc.)
	uc.requestsByHeaders = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "caddy_usage_requests_by_headers_total",
			Help: "Total number of requests by specific header values",
		},
		[]string{"header_name", "header_value", "method", "status_code"},
	)

	// Request duration histogram
	uc.requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "caddy_usage_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "status_code", "host"},
	)
}

// registerAllMetrics registers all metrics with Prometheus, handling duplicates gracefully
func (uc *UsageCollector) registerAllMetrics() error {
	metrics := []prometheus.Collector{
		uc.requestsTotal,
		uc.requestsByIP,
		uc.requestsByURL,
		uc.requestsByHeaders,
		uc.requestDuration,
	}

	for i, metric := range metrics {
		if err := uc.registerSingleMetric(metric, i); err != nil {
			return err
		}
	}

	return nil
}

// registerSingleMetric registers a single metric and handles duplicate registration
func (uc *UsageCollector) registerSingleMetric(metric prometheus.Collector, index int) error {
	if err := uc.registry.Register(metric); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			uc.logger.Debug("metric already registered, using existing", zap.Error(err))
			uc.replaceWithExistingMetric(are, index)
			return nil
		}
		return fmt.Errorf("failed to register metric: %w", err)
	}
	return nil
}

// replaceWithExistingMetric replaces our metric with the existing registered one
func (uc *UsageCollector) replaceWithExistingMetric(are prometheus.AlreadyRegisteredError, index int) {
	switch index {
	case 0:
		uc.replaceRequestsTotal(are)
	case 1:
		uc.replaceRequestsByIP(are)
	case 2:
		uc.replaceRequestsByURL(are)
	case 3:
		uc.replaceRequestsByHeaders(are)
	case 4:
		uc.replaceRequestDuration(are)
	}
}

func (uc *UsageCollector) replaceRequestsTotal(are prometheus.AlreadyRegisteredError) {
	if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
		uc.requestsTotal = existing
	}
}

func (uc *UsageCollector) replaceRequestsByIP(are prometheus.AlreadyRegisteredError) {
	if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
		uc.requestsByIP = existing
	}
}

func (uc *UsageCollector) replaceRequestsByURL(are prometheus.AlreadyRegisteredError) {
	if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
		uc.requestsByURL = existing
	}
}

func (uc *UsageCollector) replaceRequestsByHeaders(are prometheus.AlreadyRegisteredError) {
	if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
		uc.requestsByHeaders = existing
	}
}

func (uc *UsageCollector) replaceRequestDuration(are prometheus.AlreadyRegisteredError) {
	if existing, ok := are.ExistingCollector.(*prometheus.HistogramVec); ok {
		uc.requestDuration = existing
	}
}

// ServeHTTP implements the HTTP handler interface. This is where we collect
// metrics at the end of the request cycle to avoid interfering with the request.
func (uc *UsageCollector) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Record start time for duration calculation
	startTime := time.Now()

	// Create a response recorder to capture status code
	rec := caddyhttp.NewResponseRecorder(w, nil, nil)

	// Continue with the next handler in the chain
	err := next.ServeHTTP(rec, r)

	// Collect metrics after the request has been processed
	uc.collectMetrics(rec, r, startTime)

	return err
}

// collectMetrics gathers all the comprehensive metrics from the completed request
func (uc *UsageCollector) collectMetrics(rec caddyhttp.ResponseRecorder, r *http.Request, startTime time.Time) {
	// Calculate request duration
	duration := time.Since(startTime).Seconds()

	// Get basic request information
	statusCode := strconv.Itoa(rec.Status())
	method := r.Method
	host := r.Host
	path := r.URL.Path
	fullURL := r.URL.String()
	clientIP := getClientIP(r)

	// Update basic request metrics
	uc.requestsTotal.WithLabelValues(statusCode, method, host, path).Inc()
	uc.requestsByIP.WithLabelValues(clientIP, statusCode, method).Inc()
	uc.requestsByURL.WithLabelValues(fullURL, method, statusCode).Inc()
	uc.requestDuration.WithLabelValues(method, statusCode, host).Observe(duration)

	// Collect metrics for important headers
	uc.collectHeaderMetrics(r, method, statusCode)

	// Log debug information
	uc.logger.Debug("collected usage metrics",
		zap.String("client_ip", clientIP),
		zap.String("method", method),
		zap.String("url", fullURL),
		zap.String("status", statusCode),
		zap.Float64("duration", duration),
	)
}

// collectHeaderMetrics extracts and records metrics for important HTTP headers
func (uc *UsageCollector) collectHeaderMetrics(r *http.Request, method, statusCode string) {
	// List of headers we want to track
	importantHeaders := []string{
		"User-Agent",
		"Referer",
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
		"Content-Type",
		"Authorization", // Note: We'll hash this for security
		"X-Forwarded-For",
		"X-Real-IP",
		"Origin",
	}

	for _, headerName := range importantHeaders {
		headerValue := r.Header.Get(headerName)
		if headerValue != "" {
			// For sensitive headers like Authorization, we'll just track presence
			if headerName == "Authorization" {
				headerValue = "present"
			}

			// Truncate very long header values to prevent label explosion
			if len(headerValue) > 100 {
				headerValue = headerValue[:100] + "..."
			}

			uc.requestsByHeaders.WithLabelValues(headerName, headerValue, method, statusCode).Inc()
		}
	}
}

// getClientIP extracts the real client IP address from the request,
// checking various headers that might contain the original IP
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (most common for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		if ips := strings.Split(xff, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Check X-Forwarded header
	if xf := r.Header.Get("X-Forwarded"); xf != "" {
		return xf
	}

	// Fall back to RemoteAddr
	if ip := strings.Split(r.RemoteAddr, ":"); len(ip) > 0 {
		return ip[0]
	}

	return r.RemoteAddr
}

// Validate implements caddy.Validator to ensure the module configuration is valid
func (uc *UsageCollector) Validate() error {
	return nil
}

// parseCaddyfile parses the Caddyfile configuration for the usage directive
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var uc UsageCollector

	// The usage directive doesn't require any configuration parameters
	// It automatically collects metrics when Caddy's metrics are enabled
	for h.Next() {
		// No additional configuration needed
		if h.NextArg() {
			return nil, h.ArgErr()
		}
	}

	return &uc, nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler for JSON configuration
func (uc *UsageCollector) UnmarshalCaddyfile(_ *caddyfile.Dispenser) error {
	// No configuration needed - the module works automatically
	// when Caddy's metrics system is enabled
	return nil
}

// Interface guards to ensure we implement the required interfaces
var (
	_ caddy.Provisioner           = (*UsageCollector)(nil)
	_ caddy.Validator             = (*UsageCollector)(nil)
	_ caddyhttp.MiddlewareHandler = (*UsageCollector)(nil)
	_ caddyfile.Unmarshaler       = (*UsageCollector)(nil)
)
