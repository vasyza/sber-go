.PHONY: build test race vet fmt fmt-check check

build:
	go build ./...
	go build -o bin/sber ./cmd/sber
	go build -o bin/rental-check ./cmd/rental-check

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }

check: fmt-check vet race build
	go mod verify
