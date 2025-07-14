# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go backend application for the "Rinha de Backend 2025" challenge. The project implements a payment processing intermediary that connects to two payment processor services (default and fallback) with different fee rates.

## Architecture

The application follows a layered architecture:

- **cmd/api/main.go**: Application entry point with graceful shutdown handling
- **internal/server/**: HTTP server setup using Echo framework
- **internal/database/**: Database service layer with PostgreSQL integration
- **payment-processor/**: External payment processor services with Docker setup

### Key Components

- **Echo Framework**: Used for HTTP routing and middleware
- **PostgreSQL**: Primary database with connection pooling
- **Docker**: Containerized services with network connectivity between payment processors
- **Testcontainers**: Integration testing with containerized PostgreSQL

## Development Commands

Use the Makefile for all development tasks:

- `make build` - Build the application binary
- `make run` - Run the application directly
- `make test` - Run unit tests
- `make itest` - Run integration tests (database layer)
- `make watch` - Live reload development (requires air)
- `make docker-run` - Start database container
- `make docker-down` - Stop database container
- `make clean` - Remove build artifacts
- `make all` - Build and test

## Environment Configuration

The application uses environment variables defined in `.env`:

- `PORT`: Server port (default 8080)
- `BLUEPRINT_DB_*`: Database connection parameters
- Required for payment processor integration:
  - `PAYMENT_PROCESSOR_URL_DEFAULT=http://payment-processor-default:8080`
  - `PAYMENT_PROCESSOR_URL_FALLBACK=http://payment-processor-fallback:8080`

## Docker Network Setup

The application connects to external payment processors via the `payment-processor` Docker network:

```yaml
networks:
  payment-processor:
    external: true
    name: payment-processor
```

Start payment processors first: `cd payment-processor && docker compose -f docker-compose-arm64.yml up` (for ARM64 Macs)

## API Requirements

The application must implement:

- `POST /payments` - Accept payment requests with correlationId and amount
- `GET /payments-summary` - Return payment summary by processor type with optional date filtering

Integration with payment processors:
- Default processor: `http://payment-processor-default:8080/payments` (lower fees)
- Fallback processor: `http://payment-processor-fallback:8080/payments` (higher fees)
- Health check: `GET /payments/service-health` (rate limited to 1 call per 5 seconds)

## Testing

- Unit tests: Standard Go testing
- Integration tests: Use testcontainers for database testing
- Test database isolation: Each test gets a fresh PostgreSQL container

## Resource Constraints

Docker compose services must not exceed:
- Total CPU: 1.5 units
- Total Memory: 350MB
- Minimum 2 web server instances required
- Must expose API on port 9999 for testing

## Performance Strategy

The challenge requires optimizing for:
- Fastest payment processing
- Lowest transaction fees (prefer default processor)
- Handling processor instabilities (timeouts, 5XX errors)
- Async processing capabilities for better throughput