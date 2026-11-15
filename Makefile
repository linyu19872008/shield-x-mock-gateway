GO ?= go
BIN ?= bin/mock-gateway

.PHONY: help build test vet conformance clean

help:
	@echo "make build       编译 bin/mock-gateway"
	@echo "make test        单元测试"
	@echo "make vet         go vet"
	@echo "make conformance 跑模板渲染一致性向量"

build:
	$(GO) build -o $(BIN) ./cmd/mock-gateway

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

conformance:
	$(GO) run ./cmd/mock-gateway conformance --vectors ./testdata/template-conformance/cases.json

clean:
	rm -f $(BIN)
