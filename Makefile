BINARY   = eclipseir
GOFILES  = $(wildcard *.go)
GOOS     = linux
GOARCH   = arm64

.PHONY: all build clean test examples fmt vet

all: build

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BINARY) .
	@echo "Built: ./$(BINARY)"

# Build for host machine (for testing on x86 dev box)
build-host:
	go build -o $(BINARY) .
	@echo "Built (host): ./$(BINARY)"

fmt:
	gofmt -w *.go

vet:
	go vet ./...

clean:
	rm -f $(BINARY) *.s *.o examples/*.s examples/*.o

# Run IR dumps on examples (requires host build)
test: build-host
	@echo "── hello.ir ──────────────────────────────────"
	./$(BINARY) examples/hello.ir --emit-ir --stats --dry-run --verbose
	@echo ""
	@echo "── fib.ir ────────────────────────────────────"
	./$(BINARY) examples/fib.ir --emit-ir --stats --dry-run --verbose
	@echo ""
	@echo "── syscall_demo.ir ───────────────────────────"
	./$(BINARY) examples/syscall_demo.ir --emit-ir --stats --dry-run --verbose

# Emit assembly for all examples
examples: build-host
	./$(BINARY) examples/hello.ir --emit-asm --output-dir examples/ --verbose
	./$(BINARY) examples/fib.ir   --emit-asm --output-dir examples/ --verbose
	./$(BINARY) examples/syscall_demo.ir --emit-asm --output-dir examples/ --verbose

# Full build on AArch64 Linux target
target-build: build
	./$(BINARY) examples/hello.ir --out examples/hello --verbose
