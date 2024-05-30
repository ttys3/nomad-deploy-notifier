package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/nomad/api"

	"github.com/ttys3/nomad-event-notifier/internal/bot"
	"github.com/ttys3/nomad-event-notifier/internal/stream"
	"github.com/ttys3/nomad-event-notifier/version"
)

func main() {
	fmt.Printf("%s %s %s\n", version.ServiceName, version.Version, version.BuildTime)
	os.Exit(realMain())
}

func realMain() int {
	ctx, closer := CtxWithInterrupt(context.Background())
	defer closer()

	botCfg := bot.Config{
		Token:      os.Getenv("SLACK_TOKEN"),
		Channel:    os.Getenv("SLACK_CHANNEL"),
		WebhookURL: os.Getenv("DISCORD_WEBHOOK_URL"),
	}

	config := api.DefaultConfig()
	s, err := stream.NewStream(config)
	if err != nil {
		panic(err)
	}

	s.L.Info("new stream created", "config", config)

	// for user click in Slack to open the link
	nomadServerExternalURL := os.Getenv("NOMAD_SERVER_EXTERNAL_URL")
	if nomadServerExternalURL == "" {
		nomadServerExternalURL = config.Address
		s.L.Info("using default nomad server external URL since NOMAD_SERVER_EXTERNAL_URL is empty",
			"nomad_url", nomadServerExternalURL)
	}

	b, err := bot.NewBot(botCfg, nomadServerExternalURL)
	if err != nil {
		panic(err)
	}
	s.L.Info("new slack bot created", "botCfg", botCfg)

	s.L.Info("begin subscribe event stream")
	s.Subscribe(ctx, b)
	s.L.Info("end subscribe event stream")

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
