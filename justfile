app_name := "webhook"

build:
  go build -o bin/{{app_name}} -ldflags="-s -w" ./cmd

run *ARGS: build
	@./bin/{{app_name}} {{ARGS}}

simulate *ARGS: (run "simulate" ARGS)

test:
  go test ./...

test-integration:
  go test ./pkg/cache -integration