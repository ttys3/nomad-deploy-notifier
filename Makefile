NAME := nomad-event-notifier
TAG := $(shell git describe --always --tags --abbrev=0 | tr -d "[v\r\n]")
COMMIT := $(shell git rev-parse --short HEAD| tr -d "[ \r\n\']")
VERSION :=v$(TAG)-$(COMMIT)
BUILD_TIME := $(shell date +%Y%m%d-%H%M%S)

VERSION_PKG := github.com/ttys3/nomad-event-notifier/version
LD_FLAGS := "-w -s -X $(VERSION_PKG).ServiceName=$(NAME) -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME)"

all: bin

bin:
	CGO_ENABLED=0 go build -ldflags=$(LD_FLAGS) ./cmd/nomad-event-notifier/

clean:
	-rm -f nomad-event-notifier

podman/build: $(BIN)
	podman build -t $(NAME):$(TAG) -f Dockerfile .

podman/push: podman/build
	podman push $(NAME):$(TAG) docker.io/80x86/$(NAME):$(TAG)
