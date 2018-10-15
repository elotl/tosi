GIT_N_COMMITS=$(shell git log --oneline | wc -l)
GIT_REVISION=$(shell git log --pretty=format:'%h' -n 1)
GIT_DIRTY=$(shell git diff-index --quiet HEAD || echo -n '-dirty')
VERSION=$(GIT_N_COMMITS)-$(GIT_REVISION)$(GIT_DIRTY)

LDFLAGS=-ldflags "-X main.VERSION=$(VERSION)"

BINARIES=tosi

TOP_DIR=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
CMD_SRC=$(shell find $(TOP_DIR)cmd -type f -name '*.go')
VENDOR_SRC=$(shell find $(TOP_DIR)vendor -type f -name '*.go')

all: $(BINARIES)

tosi: $(PKG_SRC) $(VENDOR_SRC) $(CMD_SRC)
	cd cmd/tosi && go build $(LDFLAGS) -o $(TOP_DIR)/tosi

clean:
	rm -f $(BINARIES)

.PHONY: all clean install
