BINARY  := tachtop
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)
PREFIX  ?= /usr/local
BINDIR  ?= $(PREFIX)/bin
DATADIR ?= $(PREFIX)/share

GO      ?= go
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build run test vet fmt install install-desktop uninstall clean

all: build

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/tachtop

run: build
	./$(BINARY)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w ./cmd ./internal

install: build
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)

install-desktop:
	install -d $(DESTDIR)$(DATADIR)/applications
	install -m 0644 contrib/tachtop.desktop $(DESTDIR)$(DATADIR)/applications/tachtop.desktop

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY)
	rm -f $(DESTDIR)$(DATADIR)/applications/tachtop.desktop

clean:
	rm -f $(BINARY)
