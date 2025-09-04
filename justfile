build:
  go build -o bin/webhook -ldflags="-s" ./cmd

run:
  ./bin/webhook

test:
  go test ./...