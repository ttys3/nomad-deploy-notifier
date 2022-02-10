all: bin

bin:
	go build ./cmd/nomad-event-notifier/


clean:
	-rm -f nomad-event-notifier
