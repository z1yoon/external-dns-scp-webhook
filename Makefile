.PHONY: build test lint docker

build:
	go build ./...

test:
	go test ./...

lint:
	go vet ./...

docker:
	docker build -t external-dns-scp-webhook:dev .
