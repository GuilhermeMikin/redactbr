# Development commands. Nothing here installs tools: golangci-lint and govulncheck must already be on PATH
# (CONTRIBUTING.md shows how to install the versions CI uses).

GO       ?= go
FUZZTIME ?= 20s

.PHONY: check fmt fmt-check vet lint test race bench fuzz vuln examples fidelity

check: fmt-check vet lint test race examples vuln

fmt:
	gofmt -w .

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt would change:"; echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

bench:
	$(GO) test -run='^$$' -bench=. -benchmem ./...

# Runs every fuzz target of every package for FUZZTIME each. New failing inputs are written to
# testdata/fuzz/ by the Go toolchain; commit them as regression seeds after fixing the bug.
fuzz:
	@for pkg in $$($(GO) list ./...); do \
		for target in $$($(GO) test -list '^Fuzz' $$pkg | grep '^Fuzz'); do \
			echo "== $$pkg $$target ($(FUZZTIME))"; \
			$(GO) test -run='^$$' -fuzz="^$$target$$" -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
		done; \
	done

vuln:
	govulncheck ./...

examples:
	$(GO) run ./examples/basic
	$(GO) run ./examples/llm-stream
	$(GO) run ./examples/slog

# Runs the token fidelity harness offline, against simulated models. It sends nothing; see docs/token-fidelity.md.
fidelity:
	$(GO) run ./internal/tokenfidelity
