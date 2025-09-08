# Webhook Service

A webhook service for the ZINC system built with Go and the Echo web framework. This service handles webhook triggers and provides user authentication with JWT support.

## Features

- **Webhook Processing**: Core webhook trigger handling and grading processing
- **JWT Authentication**: RS256 algorithm with JWK endpoint support
- **GraphQL Integration**: User service communication with external GraphQL API
- **Redis Caching**: Redis-based caching with mock support for testing
- **Docker Support**: Multi-stage builds using Chainguard base images
- **Dependency Injection**: Clean architecture using Uber's fx framework

## Quick Start

### Prerequisites

- Go 1.24+
- Redis (for caching)
- Docker (optional)

### Installation

```bash
# Clone the repository
git clone https://github.com/zinc-sig/webhook.git
cd webhook

# Build the application
just build
# or directly:
go build -o bin/webhook -ldflags="-s -w" ./cmd
```

### Running the Service

```bash
# Run the built binary
just run
# or directly:
./bin/webhook serve

# Run in development mode with debug logging
./bin/webhook serve --dev
```

The service will start on port 4000 by default.

### Using Docker

```bash
# Build Docker image
docker build -t webhook .

# Run the container
docker run -p 4000:4000 webhook serve
```

## Development

### Available Commands

```bash
# Build with optimizations
just build

# Run the service
just run

# Run all tests
just test

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test ./pkg/trigger/...
```

### Architecture

The application uses a modular architecture with dependency injection:

- **`cmd/`**: CLI entry point using Cobra framework
- **`pkg/app`**: Application bootstrap and fx module composition
- **`pkg/api`**: HTTP router setup using Echo framework
- **`pkg/trigger`**: Core webhook trigger handling and grading processing
- **`pkg/user`**: User service with JWT authentication and GraphQL integration
- **`pkg/cache`**: Redis-based caching service
- **`pkg/repository`**: Data access layer with user, course, and submission repositories
- **`pkg/auth`**: Authentication utilities

### API Endpoints

- `/` - Health check endpoint
- `/trigger/*` - Webhook trigger endpoints
- User authentication endpoints

### Testing

The project uses:
- **testify** for assertions
- **Mock implementations** in `pkg/mock` for GraphQL client testing
- **Redis mocking** via go-redis/redismock for cache testing
- **testcontainers** for integration testing

## Configuration

The service can be configured via:
- Configuration file (default: `$XDG_CONFIG_DIR/config.yaml`)
- Environment variables
- Command-line flags

## CI/CD

GitHub Actions workflow automatically:
- Runs tests on all branches
- Builds and pushes Docker images to GitHub Container Registry on main branch
- Uses Go 1.24 for builds

## Version

Current version: **1.5**

Check version with:
```bash
./bin/webhook --version
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Ensure all tests pass: `go test ./...`
6. Submit a pull request

## License

[Add your license information here]