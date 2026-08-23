BINARY=horselens
CMD=./cmd/horselens

build:
	go build -o $(BINARY) $(CMD)

install:
	go install $(CMD)

run:
	go run $(CMD)

clean:
	rm -f $(BINARY)

test:
	go test ./...

lint:
	go vet ./...
