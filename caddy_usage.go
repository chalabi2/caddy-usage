package caddyusage

import (
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

// usageMetrics holds all the usage metrics
type usageMetrics struct {
	requestsTotal     *prometheus.CounterVec
	requestsByIP      *prometheus.CounterVec
	requestsByURL     *prometheus.CounterVec
	requestsByHeaders *prometheus.CounterVec
	requestDuration   *prometheus.HistogramVec
}

var (
	// Global metrics instance
	globalUsageMetrics *usageMetrics
)

// initializeMetrics creates and registers all usage metrics with Caddy's metrics registry
func initializeMetrics(registry prometheus.Registerer) (*usageMetrics, error) {
	const ns, sub = "caddy", "usage"

	metrics := &usageMetrics{
		// Total requests by status code, method, and host
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Subsystem: sub,
				Name:      "requests_total",
				Help:      "Total number of HTTP requests by status code, method, and host",
			},
			[]string{"status_code", "method", "host", "path"},
		),

		// Requests by client IP address
		requestsByIP: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Subsystem: sub,
				Name:      "requests_by_ip_total",
				Help:      "Total number of requests by client IP address",
			},
			[]string{"client_ip", "status_code", "method"},
		),

		// Requests by exact URL path and query parameters
		requestsByURL: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Subsystem: sub,
				Name:      "requests_by_url_total",
				Help:      "Total number of requests by exact URL path and query parameters",
			},
			[]string{"full_url", "method", "status_code"},
		),

		// Requests by specific headers (User-Agent, Referer, etc.)
		requestsByHeaders: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Subsystem: sub,
				Name:      "requests_by_headers_total",
				Help:      "Total number of requests by specific header values",
			},
			[]string{"header_name", "header_value", "method", "status_code"},
		),

		// Request duration histogram
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Subsystem: sub,
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "status_code", "host"},
		),
	}

	// Register each metric with Caddy's registry
	collectors := []prometheus.Collector{
		metrics.requestsTotal,
		metrics.requestsByIP,
		metrics.requestsByURL,
		metrics.requestsByHeaders,
		metrics.requestDuration,
	}

	for _, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			// Check if it's already registered error, which is expected on config reload
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				// If it's not an AlreadyRegisteredError, return the actual error
				return nil, err
			}
			// If it's AlreadyRegisteredError, continue - this is expected
		}
	}

	return metrics, nil
}

// registerMetrics registers all usage metrics with the provided Prometheus registry
func registerMetrics(registry prometheus.Registerer) error {
	// Try to initialize metrics - may handle AlreadyRegisteredError gracefully
	metrics, err := initializeMetrics(registry)
	if err != nil {
		return err
	}

	// Set the global metrics instance if it's nil
	// On config reload, this ensures we continue using metrics even if some were already registered
	if globalUsageMetrics == nil {
		globalUsageMetrics = metrics
	}

	return nil
}

// UsageCollector is a Caddy HTTP handler that collects comprehensive request metrics
// and integrates them with Caddy's built-in metrics system. It tracks response status codes,
// client IPs, requested URLs, and request headers.
type UsageCollector struct {
	logger *zap.Logger
	ctx    caddy.Context
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
	uc.ctx = ctx
	uc.logger = ctx.Logger(uc)

	// Register metrics with Caddy's internal metrics registry
	if registry := ctx.GetMetricsRegistry(); registry != nil {
		if err := registerMetrics(registry); err != nil {
			uc.logger.Warn("failed to register usage metrics", zap.Error(err))
		}
	} else {
		uc.logger.Warn("metrics registry not available, disabling metrics")
	}

	uc.logger.Info("usage collector provisioned successfully")
	return nil
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

	// Write the recorded response back to the client
	if writeErr := rec.WriteResponse(); writeErr != nil {
		uc.logger.Warn("failed to write response", zap.Error(writeErr))
	}

	// Collect metrics after the request has been processed
	uc.collectMetrics(rec, r, startTime)

	return err
}

// collectMetrics gathers all the comprehensive metrics from the completed request
func (uc *UsageCollector) collectMetrics(rec caddyhttp.ResponseRecorder, r *http.Request, startTime time.Time) {
	// Use global metrics instance
	if globalUsageMetrics == nil {
		uc.logger.Error("usage metrics not initialized")
		return
	}

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

	globalUsageMetrics.requestsTotal.WithLabelValues(statusCode, method, host, path).Inc()
	globalUsageMetrics.requestsByIP.WithLabelValues(clientIP, statusCode, method).Inc()
	globalUsageMetrics.requestsByURL.WithLabelValues(fullURL, method, statusCode).Inc()
	globalUsageMetrics.requestDuration.WithLabelValues(method, statusCode, host).Observe(duration)

	// Collect metrics for important headers
	uc.collectHeaderMetrics(globalUsageMetrics, r, method, statusCode)
}

// collectHeaderMetrics extracts and records metrics for important HTTP headers
func (uc *UsageCollector) collectHeaderMetrics(um *usageMetrics, r *http.Request, method, statusCode string) {
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

			um.requestsByHeaders.WithLabelValues(headerName, headerValue, method, statusCode).Inc()
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
	// Handle IPv6 addresses which are in format [::1]:port
	if strings.HasPrefix(r.RemoteAddr, "[") {
		if endBracket := strings.Index(r.RemoteAddr, "]"); endBracket != -1 {
			return r.RemoteAddr[:endBracket+1]
		}
	}

	// Handle IPv4 addresses in format ip:port
	if ip := strings.Split(r.RemoteAddr, ":"); len(ip) > 0 {
		return ip[0]
	}

	return r.RemoteAddr
}

// Cleanup cleans up the handler, following caddy-ratelimit pattern
func (uc *UsageCollector) Cleanup() error {
	// Note: We don't delete metrics from the pool here because they might be used
	// by other instances. Metrics will be cleaned up when the process exits.
	return nil
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
	_ caddy.CleanerUpper          = (*UsageCollector)(nil)
	_ caddyhttp.MiddlewareHandler = (*UsageCollector)(nil)
	_ caddyfile.Unmarshaler       = (*UsageCollector)(nil)
)
