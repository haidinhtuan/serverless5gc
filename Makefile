.PHONY: test test-unit test-integration build lint clean

MODULE := github.com/tdinh/serverless5gc

test-unit:
	go test ./... -v -count=1

test-integration:
	go test ./... -v -count=1 -tags=integration

test: test-unit

build-proxy:
	go build -o bin/sctp-proxy ./cmd/sctp-proxy/

build-functions:
	@for dir in functions/*/; do \
		echo "Building $$dir..."; \
	done

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
