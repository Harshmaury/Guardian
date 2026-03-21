# Makefile — Guardian
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X main.guardianVersion=$(VERSION)
BINDIR  := $(HOME)/bin

.PHONY: all build clean

all: build

build:
	@echo "  → guardian $(VERSION)"
	@CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/guardian ./cmd/guardian/

clean:
	@rm -f $(BINDIR)/guardian
