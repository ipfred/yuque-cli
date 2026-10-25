VERSION ?= 0.1.2
PREFIX  ?= $(HOME)/.local
LDFLAGS := -s -w -X yuque-cli/internal/cli.Version=$(VERSION)

.PHONY: build test install uninstall dist clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/yuque-cli ./cmd/yuque-cli

test:
	go vet ./...
	go test ./...

# 安装二进制到 $(PREFIX)/bin,并把 skill 装进本机已有的 AI 客户端。
install: build
	mkdir -p $(PREFIX)/bin
	install -m 0755 bin/yuque-cli $(PREFIX)/bin/yuque-cli
	$(PREFIX)/bin/yuque-cli skill install --yes
	@echo
	@echo "已安装到 $(PREFIX)/bin/yuque-cli"
	@echo "$$PATH" | tr ':' '\n' | grep -qx "$(PREFIX)/bin" || echo "提示: 请把 $(PREFIX)/bin 加入 PATH"
	@echo "下一步: yuque-cli auth login <任意一篇语雀文档链接>"

uninstall:
	-$(PREFIX)/bin/yuque-cli skill uninstall --yes
	rm -f $(PREFIX)/bin/yuque-cli

# 交叉编译,产物在 dist/,命名与 CI Release 资产一致(scripts/install.sh 依赖此命名)
dist:
	rm -rf dist && mkdir -p dist
	for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
		os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=".exe"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" \
			-o dist/yuque-cli_$(VERSION)_$${os}_$${arch}$$ext ./cmd/yuque-cli || exit 1; \
	done
	ls -l dist

clean:
	rm -rf bin dist
