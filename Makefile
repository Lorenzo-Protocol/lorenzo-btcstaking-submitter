BUILDDIR ?= $(CURDIR)/build
BIN_NAME=lrz-btcstaking-submitter

build:
	go build -o $(BUILDDIR)/$(BIN_NAME) .
clean:
	rm -rf $(BUILDDIR)

.PHONY: build