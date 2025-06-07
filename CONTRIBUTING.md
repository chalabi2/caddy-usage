# Contributing

Contributions are welcome! Please follow these guidelines:

## Development Setup

1. **Fork and clone the repository:**

```bash
bash -c "git clone https://github.com/your-username/caddy-usage"
bash -c "cd caddy-usage"
```

2. **Install dependencies:**

```bash
bash -c "make deps"
```

3. **Build with xcaddy:**

```bash
bash -c "make xcaddy-build"
```

## Making Changes

1. **Create a feature branch:**

```bash
bash -c "git checkout -b feature/your-feature-name"
```

2. **Make your changes and ensure tests pass:**

```bash
bash -c "make test"
bash -c "make race"
bash -c "make benchmark"
```

3. **Run code quality checks:**

```bash
bash -c "make check"
```

4. **Test with a Caddy instance:**

```bash
bash -c "make xcaddy-run"
```

### Code Style

- Follow Go conventions and use `make fmt` to format code
- Add comprehensive tests for new features
- Update documentation for any API changes
- Ensure Prometheus metrics follow naming conventions
- Handle errors appropriately and add logging where needed
- Run `make check` before committing to ensure code quality

## Submitting Changes

1. **Commit with clear messages:**

```bash
bash -c "git commit -m 'Add feature: description of what was added'"
```

2. **Push to your fork:**

```bash
bash -c "git push origin feature/your-feature-name"
```

3. **Create a Pull Request** with:
   - Clear description of changes
   - Test results
   - Any breaking changes noted

## Testing

Before submitting, run the full test suite:

```bash
bash -c "make ci"  # Runs all CI checks locally
```

This includes:

- All unit tests (`make test`)
- Race condition detection (`make race`)
- Code quality checks (`make check`)
- Dependency verification

Additional testing requirements:

- Add tests for new functionality
- Test with both Caddyfile and JSON configurations (`make xcaddy-run`)
- Verify metrics are properly exported to Prometheus
- Test with various HTTP scenarios (different status codes, methods, headers)
- Run benchmarks for performance-sensitive changes (`make benchmark`)
