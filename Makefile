.PHONY: build clean install

BUILD_DIR := build
BINARY := $(BUILD_DIR)/emailo

build:
	@if ! command -v go >/dev/null 2>&1; then \
		echo "warning: Go is not installed."; \
		echo "Install Go, then run make build again."; \
		exit 1; \
	fi
	mkdir -p $(BUILD_DIR)
	go build -o $(BINARY) .

clean:
	rm -rf $(BUILD_DIR)

install: build
	install -d $(HOME)/.local/bin
	install -m 755 $(BINARY) $(HOME)/.local/bin/emailo
	install -d -m 700 $(HOME)/.emailo
	@if [ ! -e $(HOME)/.emailo/config ]; then \
		install -m 600 config.template $(HOME)/.emailo/config; \
	fi
