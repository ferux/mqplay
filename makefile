export GO111MODULE=on
GIT_VER?=$(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_FLAGS=-mod vendor
TEST_FLAGS=-race -timeout=60s
PKG=mqplay
PKG_PATH=github.com/ferux/$(PKG)

OUT?=./bin/mqplay

default: build_develop

clean:
	-@rm $(OUT)
.PHONY: clean

build: clean
	go build $(BUILD_FLAGS) --ldflags '-X main.revision=$(GIT_VER) -X main.buildtime=$(BUILD_TIME)' -o $(OUT) ./internal/cmd/main.go
.PHONY: build


build_production: BUILD_FLAGS+=-tags production
build_production: build
.PHONY: build_production

check:
	golangci-lint run ./internal/...
.PHONY: check

test:
	go test $(TEST_FLAGS) ./internal/...
.PHONY: test

run: build
	$(OUT)
.PHONY: run