.PHONY: build clean vet test

BINARY := berth
CMD := ./cmd/berth

build:
	go build -o $(BINARY) $(CMD)

clean:
	rm -f $(BINARY)

vet:
	go vet ./...

test:
	go test ./...
