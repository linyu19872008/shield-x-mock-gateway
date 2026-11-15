GO ?= go
BIN ?= bin/mock-gateway
VERSION ?= 0.1.1
LDFLAGS ?= -X github.com/linyu19872008/shield-x-mock-gateway/internal/version.Version=$(VERSION)

.PHONY: help build test vet conformance i18n-check docs-check clean

help:
	@echo "make build       编译 bin/mock-gateway"
	@echo "make test        单元测试"
	@echo "make vet         go vet"
	@echo "make conformance 跑模板渲染一致性向量"
	@echo "make i18n-check  检查中英文 UI 词典和 key"
	@echo "make docs-check  检查文档相对链接和双语镜像"

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/mock-gateway

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

conformance:
	$(GO) run ./cmd/mock-gateway conformance --vectors ./testdata/template-conformance/cases.json

i18n-check:
	python3 scripts/check-i18n.py
	node --check internal/control/web/app.js
	node --check internal/control/web/i18n.js
	node --check internal/control/web/locales/en.js
	node --check internal/control/web/locales/zh-CN.js

docs-check:
	python3 scripts/check-docs.py

clean:
	rm -f $(BIN)
