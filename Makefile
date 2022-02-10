all: bin

bin:
	go build ./cmd/bot/


clean:
	-rm -f bot
