.PHONY: build test fmt vet tidy run

build:
	go build ./...

test:
	go test -timeout=300s -count=1 ./...

race:
	go test -race -timeout=420s -count=1 ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

run:
	go run ./cmd/voltforge

ctl:
	go run ./cmd/voltforgectl status
