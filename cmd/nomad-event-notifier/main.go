package main

import (
	"context"
	"fmt"
	"github.com/ttys3/nomad-event-notifier/version"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/nomad/api"
	"github.com/ttys3/nomad-event-notifier/internal/bot"
	"github.com/ttys3/nomad-event-notifier/internal/stream"
)

func main() {
	fmt.Printf("%s %s %s\n", version.ServiceName, version.Version, version.BuildTime)
	os.Exit(realMain(os.Args))
}

func realMain(args []string) int {
	ctx, closer := CtxWithInterrupt(context.Background())
	defer closer()

	token := os.Getenv("SLACK_TOKEN")
	toChannel := os.Getenv("SLACK_CHANNEL")

	slackCfg := bot.Config{
		Token:   token,
		Channel: toChannel,
	}

	config := api.DefaultConfig()
	stream := stream.NewStream(config)
	stream.L.Info("new stream created", "config", config)

	slackBot, err := bot.NewBot(slackCfg, config.Address)
	if err != nil {
		panic(err)
	}
	stream.L.Info("new slack bot created", "slackCfg", slackCfg)

	stream.L.Info("begin subscribe event stream")
	stream.Subscribe(ctx, slackBot)
	stream.L.Info("end subscribe event stream")

	return 0
}

func CtxWithInterrupt(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	return ctx, func() {
		signal.Stop(ch)
		cancel()
	}
}
