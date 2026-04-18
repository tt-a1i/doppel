.PHONY: build test vet run clean

BIN := doppel

build:
	go build -o $(BIN) ./cmd/doppel

test:
	go test ./...

vet:
	go vet ./...

run: build
	./$(BIN)

clean:
	rm -f $(BIN)
