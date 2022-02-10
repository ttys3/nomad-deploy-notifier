all: bin

bin:
	CGO_ENABLED=0 go build ./cmd/nomad-event-notifier/

clean:
	-rm -f nomad-event-notifier
