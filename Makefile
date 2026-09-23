.PHONY: build run frontend test tidy deps

export CGO_ENABLED=1
export GOTOOLCHAIN=local
CGO_LDFLAGS ?= -L$(CURDIR)/third_party/tokenizers
export CGO_LDFLAGS

build:
	go build -o bin/laya-decision-web ./cmd/server

run: build
	./bin/laya-decision-web

frontend:
	cd frontend && npm install && npm run dev

frontend-build:
	cd frontend && npm install && npm run build

test:
	go test ./... -count=1 -timeout 15m

test-short:
	go test ./... -short -count=1

# Full Laya integration on Neural Engine (can be slow / flaky if ANE daemon is busy).
test-laya-ane:
	LAYA_TEST_ANE=1 go test ./internal/decision/laya/ -count=1 -timeout 20m -v

tidy:
	go mod tidy

# Download HuggingFace tokenizers static lib (darwin-arm64). Uses HTTPS_PROXY if set.
deps:
	mkdir -p third_party/tokenizers
	curl -fsSL -o third_party/tokenizers/libtokenizers.darwin-arm64.tar.gz \
	  https://github.com/daulet/tokenizers/releases/download/v1.27.0/libtokenizers.darwin-arm64.tar.gz
	tar -xzf third_party/tokenizers/libtokenizers.darwin-arm64.tar.gz -C third_party/tokenizers
