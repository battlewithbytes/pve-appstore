BINARY    := pve-appstore
MODULE    := github.com/battlewithbytes/pve-appstore
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -X $(MODULE)/internal/version.Version=$(VERSION) \
             -X $(MODULE)/internal/version.Commit=$(COMMIT) \
             -X $(MODULE)/internal/version.Date=$(DATE)

.PHONY: build test test-apps vet fmt lint install deploy run-install run-serve clean deps tidy release frontend

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/pve-appstore/

test:
	go test -v ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint: vet fmt

frontend:
	npm run build --prefix web/frontend

install: build
	cp $(BINARY) /opt/pve-appstore/$(BINARY)
	chmod 0755 /opt/pve-appstore/$(BINARY)

deploy: frontend build
	systemctl stop pve-appstore
	cp $(BINARY) /opt/pve-appstore/$(BINARY)
	chmod 0755 /opt/pve-appstore/$(BINARY)
	@if id appstore >/dev/null 2>&1; then chown -R appstore:appstore /var/lib/pve-appstore; fi
	systemctl start pve-appstore
	@echo "Deployed $$(/opt/pve-appstore/pve-appstore version 2>&1 | head -1)"

run-install: build
	./$(BINARY) install

test-apps: build
	./$(BINARY) test-apps --catalog-dir testdata/catalog --verbose

run-serve: build
	./$(BINARY) serve --config dev-config.yml --catalog-dir testdata/catalog

clean:
	rm -f $(BINARY)
	rm -f coverage.out coverage.html
	rm -rf web/frontend/dist

deps:
	go mod download
	npm install --prefix web/frontend

tidy:
	go mod tidy

release: frontend
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS) -s -w" -o $(BINARY)-linux-amd64 ./cmd/pve-appstore/
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS) -s -w" -o $(BINARY)-linux-arm64 ./cmd/pve-appstore/
