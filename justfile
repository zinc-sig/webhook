build:
  go build -o webhook main.go

run:
  ./webhook

test:
  go test ./...