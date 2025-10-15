build:
  go build -o bin/webhook -ldflags="-s -w" ./cmd

run: *ARGS: build
	@./bin/{{app_name}} {{ARGS}}

simulate: *ARGS: (run "simulate" ARGS)

test:
  go test ./...

test-integration:
  go test ./pkg/cache -integration