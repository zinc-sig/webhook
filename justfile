build:
  go build -o bin/webhook -ldflags="-s -w" ./cmd

run:
  ./bin/webhook

test:
  go test ./...