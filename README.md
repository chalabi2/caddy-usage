# Caddy Usage Metrics Plugin

[![codecov](https://codecov.io/gh/chalabi/caddy-usage/graph/badge.svg?token=YOUR_CODECOV_TOKEN)](https://codecov.io/gh/chalabi/caddy-usage)
[![Go Report Card](https://goreportcard.com/badge/github.com/chalabi/caddy-usage)](https://goreportcard.com/report/github.com/chalabi/caddy-usage)
[![Go Reference](https://pkg.go.dev/badge/github.com/chalabi/caddy-usage.svg)](https://pkg.go.dev/github.com/chalabi/caddy-usage)

> [!NOTE]
> This is not an official repository of the [Caddy Web Server](https://github.com/caddyserver) organization.

A comprehensive request metrics collection plugin for Caddy that integrates with Prometheus to provide detailed analytics about your web server usage.

## Features

- **Comprehensive Metrics**: Track requests by status code, method, host, path, client IP, and headers
- **Request Analytics**: Monitor URL patterns, client behavior, and traffic sources
- **Performance Monitoring**: Request duration histograms for performance analysis
- **Header Tracking**: Analyze User-Agent, Referer, and other important HTTP headers
- **Security Conscious**: Sensitive headers like Authorization are handled securely
- **Prometheus Integration**: Export metrics in Prometheus format for easy monitoring

## Installation

Build Caddy with this plugin using [xcaddy](https://github.com/caddyserver/xcaddy):

```bash
bash -c "xcaddy build --with github.com/chalabi/caddy-usage"
```

Or add to your `xcaddy.json`:

```json
{
  "dependencies": [
    {
      "module": "github.com/chalabi/caddy-usage",
      "version": "latest"
    }
  ]
}
```

## Metrics Exposed

The plugin exposes the following Prometheus metrics:

### `caddy_usage_requests_total`

**Type:** Counter  
**Description:** Total number of HTTP requests by status code, method, and host  
**Labels:**

- `status_code` - HTTP response status code (200, 404, 500, etc.)
- `method` - HTTP method (GET, POST, PUT, etc.)
- `host` - Host header value
- `path` - URL path

### `caddy_usage_requests_by_ip_total`

**Type:** Counter  
**Description:** Total number of requests by client IP address  
**Labels:**

- `client_ip` - Client's IP address (handles X-Forwarded-For, X-Real-IP)
- `status_code` - HTTP response status code
- `method` - HTTP method

### `caddy_usage_requests_by_url_total`

**Type:** Counter  
**Description:** Total number of requests by exact URL path and query parameters  
**Labels:**

- `full_url` - Complete URL with query parameters
- `method` - HTTP method
- `status_code` - HTTP response status code

### `caddy_usage_requests_by_headers_total`

**Type:** Counter  
**Description:** Total number of requests by specific header values  
**Labels:**

- `header_name` - Header name (User-Agent, Referer, Accept, etc.)
- `header_value` - Header value (truncated if > 100 chars)
- `method` - HTTP method
- `status_code` - HTTP response status code

**Tracked Headers:**

- User-Agent
- Referer
- Accept
- Accept-Language
- Accept-Encoding
- Content-Type
- Authorization (value replaced with "present" for security)
- X-Forwarded-For
- X-Real-IP
- Origin

### `caddy_usage_request_duration_seconds`

**Type:** Histogram  
**Description:** HTTP request duration in seconds  
**Labels:**

- `method` - HTTP method
- `status_code` - HTTP response status code
- `host` - Host header value

## Configuration

### Caddyfile

Simple usage - add the `usage` directive to any site or route:

```caddyfile
{
    # Enable Caddy's metrics system
    servers {
        metrics
    }
    admin localhost:2019
}

example.com {
    # Enable usage metrics collection for all requests
    usage
    file_server
}

# Or add to specific routes
api.example.com {
    route /api/* {
        usage
        reverse_proxy localhost:8080
    }

    route /health {
        usage
        respond "OK" 200
    }
}
```

### JSON Configuration

```json
{
  "admin": {
    "listen": "localhost:2019"
  },
  "metrics": {},
  "apps": {
    "http": {
      "servers": {
        "srv0": {
          "routes": [
            {
              "match": [{ "host": ["example.com"] }],
              "handle": [
                {
                  "handler": "usage"
                },
                {
                  "handler": "file_server"
                }
              ]
            }
          ]
        }
      }
    }
  }
}
```

## Usage Examples

### Basic Setup

1. **Enable metrics in your Caddyfile:**

```caddyfile
{
    servers {
        metrics
    }
}

localhost {
    usage
    respond "Hello World"
}
```

2. **Start Caddy:**

```bash
bash -c "make xcaddy-run"
```

3. **Generate some traffic:**

```bash
bash -c "curl localhost"
bash -c "curl localhost/api"
bash -c "curl -H 'User-Agent: MyBot/1.0' localhost"
```

4. **View metrics:**

```bash
bash -c "curl localhost:2019/metrics | grep caddy_usage"
```

### Sample Metrics Output

```prometheus
# HELP caddy_usage_requests_total Total number of HTTP requests by status code, method, and host
# TYPE caddy_usage_requests_total counter
caddy_usage_requests_total{host="localhost",method="GET",path="/",status_code="200"} 5
caddy_usage_requests_total{host="localhost",method="GET",path="/api",status_code="404"} 2

# HELP caddy_usage_requests_by_ip_total Total number of requests by client IP address
# TYPE caddy_usage_requests_by_ip_total counter
caddy_usage_requests_by_ip_total{client_ip="127.0.0.1",method="GET",status_code="200"} 7

# HELP caddy_usage_request_duration_seconds HTTP request duration in seconds
# TYPE caddy_usage_request_duration_seconds histogram
caddy_usage_request_duration_seconds_bucket{host="localhost",method="GET",status_code="200",le="0.005"} 5
caddy_usage_request_duration_seconds_bucket{host="localhost",method="GET",status_code="200",le="0.01"} 7
```

### Grafana Dashboard Queries

Monitor your web server with these example Prometheus queries:

```promql
# Request rate by status code
rate(caddy_usage_requests_total[5m])

# Top client IPs
topk(10, sum by (client_ip) (caddy_usage_requests_by_ip_total))

# Most popular URLs
topk(10, sum by (full_url) (caddy_usage_requests_by_url_total))

# Average request duration
avg(rate(caddy_usage_request_duration_seconds_sum[5m])) / avg(rate(caddy_usage_request_duration_seconds_count[5m]))

# Top User-Agents
topk(10, sum by (header_value) (caddy_usage_requests_by_headers_total{header_name="User-Agent"}))
```

## Requirements

- **Caddy:** v2.7.0 or higher
- **Go:** 1.21 or higher
- **Prometheus:** For metrics collection (optional)

## Building from Source

```bash
bash -c "git clone https://github.com/chalabi/caddy-usage"
bash -c "cd caddy-usage"
bash -c "make deps"
bash -c "make xcaddy-build"
```

## Testing

Run the test suite:

```bash
bash -c "make test"        # Run unit tests
bash -c "make benchmark"   # Run benchmarks
bash -c "make ci"          # Run all CI checks
```

## License

Apache License 2.0

## Bug Reports

When reporting bugs, include:

- Caddy version
- Plugin version
- Configuration (Caddyfile or JSON)
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs
