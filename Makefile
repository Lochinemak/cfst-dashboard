.PHONY: build test dist dist-agent clean

AGENT_PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

build:
	cd web && npm run build
	go build ./cmd/server ./cmd/agent

test:
	go test ./...
	cd web && npm run build

dist:
	mkdir -p dist
	cd web && npm run build
	go build -o dist/cfst-dashboard ./cmd/server
	$(MAKE) dist-agent

dist-agent:
	mkdir -p dist
	@for platform in $(AGENT_PLATFORMS); do \
		goos=$${platform%/*}; \
		goarch=$${platform#*/}; \
		ext=""; \
		if [ "$$goos" = "windows" ]; then ext=".exe"; fi; \
		echo "building dist/cfst-agent-$$goos-$$goarch$$ext"; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$goarch go build -o dist/cfst-agent-$$goos-$$goarch$$ext ./cmd/agent; \
	done

clean:
	rm -rf dist web/dist cfst-dashboard cfst-agent server agent
