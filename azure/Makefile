BUILD_TS := $(shell date -Iseconds --utc)
COMMIT_SHA := $(shell git rev-parse HEAD)
VERSION := $(shell git describe --abbrev=0 --tags)

export CGO_ENABLED=1
export GOOS=linux
export GO111MODULE=on

FILES := $(shell find . -name '*.go' -type f -not -name '*.pb.go' -not -name '*_generated.go' -not -name '*_test.go')
TESTS := $(shell find . -name '*.go' -type f -not -name '*.pb.go' -not -name '*_generated.go' -name '*_test.go')

.DEFAULT_GOAL := all
.PHONY: all
all: fmt build build-debug

fmt:
	gofmt -s -l -w $(FILES) $(TESTS)
	goimports -l -w $(FILES) $(TESTS)

.PHONY: build
build:
	go build -buildmode=plugin -o kafkactl-azure-plugin.so

.PHONY: build-debug
build-debug:
	go build -buildmode=plugin -gcflags="all=-N -l" -o kafkactl-azure-plugin_debug.so

.PHONY: clean
clean:
	rm -f kafkactl-azure-plugin.so
	go clean -testcache
