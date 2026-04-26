.PHONY: build test vet smoke smoke-fixture run clean

BIN := doppel
DOPPEL_SMOKE_LAUNCH_TEST ?= 0

build:
	go build -o $(BIN) ./cmd/doppel

test:
	go test ./...

vet:
	go vet ./...

smoke: build
	DOPPEL_SMOKE_LAUNCH_TEST=$(DOPPEL_SMOKE_LAUNCH_TEST) scripts/smoke-real-apps.sh

smoke-fixture: build
	scripts/smoke-fixture-app.sh

run: build
	./$(BIN)

clean:
	rm -f $(BIN)
