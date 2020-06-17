GIT_VERSION=$(shell git describe --dirty || echo dev)

LDFLAGS=-ldflags "-X main.Version=$(GIT_VERSION)"

BINARIES=tosi

TOP_DIR=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
CMD_SRC=$(shell find $(TOP_DIR)cmd -type f -name '*.go')
PKG_SRC=$(shell find $(TOP_DIR)pkg -type f -name '*.go')

all: $(BINARIES)

tosi: $(CMD_SRC) $(PKG_SRC) go.sum
	go build $(LDFLAGS) -o tosi cmd/tosi/tosi.go

clean:
	rm -f $(BINARIES)

.PHONY: all clean install
