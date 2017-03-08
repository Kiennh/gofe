#
#  Makefile for Go
#
GO_CMD=go
GO_BUILD=$(GO_CMD) build
GO_CLEAN=$(GO_CMD) clean

BINARY=gofe

# Packages
.PHONY: all build run clean 

all: clean build run

build:
	$(GO_BUILD) -o $(BINARY) *.go

run:
	./$(BINARY)
	
clean:
	rm $(BINARY)