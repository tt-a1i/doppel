.PHONY: build test vet run clean

BIN := appclone

build:
	go build -o $(BIN) ./cmd/appclone

test:
	go test ./...

vet:
	go vet ./...

run: build
	./$(BIN)

clean:
	rm -f $(BIN)
