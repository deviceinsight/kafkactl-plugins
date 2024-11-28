BUILD_TS := $(shell date -Iseconds --utc)
COMMIT_SHA := $(shell git rev-parse HEAD)
VERSION ?= $(shell git diff --quiet && git describe --exact-match --tags $(COMMIT_SHA) 2>/dev/null || echo "latest")

export CGO_ENABLED=0
export GOOS=linux
export GO111MODULE=on

program=kafkactl-aws-plugin
ld_flags := "-X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT_SHA) -X main.BuildTime=$(BUILD_TS)"

FILES    := $(shell find . -name '*.go' -type f -not -name '*.pb.go' -not -name '*_generated.go' -not -name '*_test.go')
TESTS    := $(shell find . -name '*.go' -type f -not -name '*.pb.go' -not -name '*_generated.go' -name '*_test.go')

.DEFAULT_GOAL := all
.PHONY: all
all: fmt lint cve-check test build

fmt:
	gofmt -s -l -w $(FILES) $(TESTS)
	goimports -l -w $(FILES) $(TESTS)

.PHONY: update-dependencies
update-dependencies: # update dependencies to latest MINOR.PATCH
	go get -t -u ./...

lint:
	golangci-lint run --config ../.golangci.yml

.PHONY: cve-check
cve-check:
	govulncheck ./...

.PHONY: test
test:
	rm -f test.log
	go test -v -short ./...

.PHONY: build
build:
	go build -ldflags $(ld_flags) -o $(program)

.PHONY: clean
clean:
	rm -f $(program)
	go clean -testcache
