VERSION ?= 1.0.0
LDFLAGS = -s -w -X main.version=$(VERSION)

.PHONY: all build test vet build-linux build-windows clean

all: test build

build:
	mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/heimdrive .

test:
	go test ./...

vet:
	go vet ./...

build-linux:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/heimdrive-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/heimdrive-linux-arm64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=386 GO386=softfloat go build -trimpath -ldflags "$(LDFLAGS)" -o dist/heimdrive-linux-386 .

build-windows:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/heimdrive-windows-amd64.exe .

clean:
	rm -rf dist
