GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Normalize GOARCH to match our lib dir naming
ifeq ($(GOARCH),amd64)
  ARCH := x86_64
else
  ARCH := $(GOARCH)
endif

PLATFORM         := $(GOOS)-$(ARCH)
LIB_DIR          := $(CURDIR)/lib/ghostty/$(PLATFORM)
PKG_CONFIG_PATH  := $(LIB_DIR)/share/pkgconfig

.PHONY: build
build:
	PKG_CONFIG_PATH="$(PKG_CONFIG_PATH)" \
	CGO_ENABLED=1 \
	go build ./cmd/dmux/

.PHONY: test
test:
	PKG_CONFIG_PATH="$(PKG_CONFIG_PATH)" \
	CGO_ENABLED=1 \
	go test ./...

.PHONY: build-linux-amd64
build-linux-amd64:
	PKG_CONFIG_PATH="$(CURDIR)/lib/ghostty/linux-x86_64/share/pkgconfig" \
	CGO_ENABLED=1 \
	GOOS=linux GOARCH=amd64 \
	CC="zig cc -target x86_64-linux-gnu" \
	go build -o dmux-linux-amd64 ./cmd/dmux/

.PHONY: build-linux-arm64
build-linux-arm64:
	PKG_CONFIG_PATH="$(CURDIR)/lib/ghostty/linux-aarch64/share/pkgconfig" \
	CGO_ENABLED=1 \
	GOOS=linux GOARCH=arm64 \
	CC="zig cc -target aarch64-linux-gnu" \
	go build -o dmux-linux-arm64 ./cmd/dmux/

.PHONY: build-windows-amd64
build-windows-amd64:
	PKG_CONFIG_PATH="$(CURDIR)/lib/ghostty/windows-x86_64/share/pkgconfig" \
	CGO_ENABLED=1 \
	GOOS=windows GOARCH=amd64 \
	CC="zig cc -target x86_64-windows-gnu" \
	go build -o dmux-windows-amd64.exe ./cmd/dmux/

.PHONY: build-windows-arm64
build-windows-arm64:
	PKG_CONFIG_PATH="$(CURDIR)/lib/ghostty/windows-aarch64/share/pkgconfig" \
	CGO_ENABLED=1 \
	GOOS=windows GOARCH=arm64 \
	CC="zig cc -target aarch64-windows-gnu" \
	go build -o dmux-windows-arm64.exe ./cmd/dmux/

.PHONY: build-darwin-amd64
build-darwin-amd64:
	PKG_CONFIG_PATH="$(CURDIR)/lib/ghostty/darwin-x86_64/share/pkgconfig" \
	CGO_ENABLED=1 \
	GOOS=darwin GOARCH=amd64 \
	CC="zig cc -target x86_64-macos" \
	go build -o dmux-darwin-amd64 ./cmd/dmux/

.PHONY: build-darwin-arm64
build-darwin-arm64:
	PKG_CONFIG_PATH="$(CURDIR)/lib/ghostty/darwin-aarch64/share/pkgconfig" \
	CGO_ENABLED=1 \
	GOOS=darwin GOARCH=arm64 \
	CC="zig cc -target aarch64-macos" \
	go build -o dmux-darwin-arm64 ./cmd/dmux/

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 build-windows-amd64 build-windows-arm64 build-darwin-amd64 build-darwin-arm64

.PHONY: rebuild-libs
rebuild-libs:
	bash scripts/build-libghostty.sh
