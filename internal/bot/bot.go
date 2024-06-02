package bot

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/hashicorp/nomad/api"
)

type Config struct {
	// You more than likely want your "Bot User OAuth Access Token" which starts with "xoxb-"
	WebhookURL string
	Token      string
	Channel    string
}

type Bot struct {
	bots []Impl
	L    *slog.Logger
}

var errImplNotEnabled = errors.New("impl not available")

type Creater = func(cfg Config, nomadAddress string) (Impl, error)

type Impl interface {
	UpsertDeployMsg(deploy api.Deployment) error
	UpsertAllocationMsg(alloc api.Allocation) error
}

func NewBot(cfg Config, nomadAddress string) (*Bot, error) {
	var bots []Impl

	for _, c := range []Creater{NewDiscordBot, newSlackBot} {
		bot, err := c(cfg, nomadAddress)
		if err != nil {
			if errors.Is(err, errImplNotEnabled) {
				continue
			}

			return nil, fmt.Errorf("failed to create bot: %w", err)
		}

		bots = append(bots, bot)
	}

	if len(bots) == 0 {
		return nil, errors.New("no bots enabled")
	}

	bot := &Bot{
		bots: bots,
		L:    slog.Default(),
	}

	return bot, nil
}

func (b *Bot) UpsertDeployMsg(deploy api.Deployment) error {
	var err error

	for _, bot := range b.bots {
		err = errors.Join(err, bot.UpsertDeployMsg(deploy))
	}

	return err
}

func (b *Bot) UpsertAllocationMsg(alloc api.Allocation) error {
	var err error

	for _, bot := range b.bots {
		err = errors.Join(err, bot.UpsertAllocationMsg(alloc))
	}

	return err
}
