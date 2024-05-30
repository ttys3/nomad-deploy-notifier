package bot

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
)

func NewDiscordBot(cfg Config, nomadAddress string) (Impl, error) {
	if cfg.WebhookURL == "" {
		return nil, fmt.Errorf("please set discord webhook url to enable discord bot: %w", errImplNotEnabled)
	}

	bot := &discordBot{
		deploys:      make(map[string]string),
		nomadAddress: nomadAddress,
		allocations:  make(map[string]string),
		client:       resty.New(),
		webhookURL:   cfg.WebhookURL,
		L:            hclog.Default().Named("discord"),
	}

	return bot, nil
}

type discordBot struct {
	mu           sync.Mutex
	nomadAddress string
	webhookURL   string
	client       *resty.Client
	deploys      map[string]string
	allocations  map[string]string
	L            hclog.Logger
}

func (b *discordBot) UpsertDeployMsg(deploy api.Deployment) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ts, ok := b.deploys[deploy.ID]
	if !ok {
		return b.initialDeployMsg(deploy)
	}
	b.L.Debug("Existing deployment found, updating status", "slack ts", ts)

	attachments := b.DefaultAttachmentsDeployment(deploy)

	var r discordgo.Message
	_, err := b.client.R().SetBody(attachments).SetResult(&r).Patch(b.webhookURL + "/messages/" + ts)
	if err != nil {
		return err
	}
	b.deploys[deploy.ID] = r.ID

	return nil
}

func (b *discordBot) initialDeployMsg(deploy api.Deployment) error {
	b.L.Info("init deploy message")

	attachments := b.DefaultAttachmentsDeployment(deploy)

	_, err := b.client.R().SetBody(attachments).Post(b.webhookURL)

	var r discordgo.Message
	res, err := b.client.R().SetBody(attachments).SetResult(&r).Post(b.webhookURL)
	if err != nil {
		return fmt.Errorf("post message failed,  err=%w", err)
	}
	if res.StatusCode() >= 300 {
		b.L.Error("failed to create message %s", string(res.Body()))
		return fmt.Errorf("failed to create message %s", res.Body())
	}

	b.deploys[deploy.ID] = r.ID
	return nil
}

func (b *discordBot) UpsertAllocationMsg(alloc api.Allocation) error {
	// do not report old OOM
	if time.Now().Unix()-alloc.ModifyTime > 300 {
		return nil
	}
	// only report last alloc OOM
	if alloc.NextAllocation != "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	ts, ok := b.allocations[alloc.ID]
	if !ok {
		return b.initialAllocMsg(alloc)
	}
	b.L.Debug("Existing allocation found, updating status", "slack ts", ts)

	attachments := b.DefaultAttachmentsAlloc(alloc)
	if len(attachments.Embeds) == 0 {
		return nil
	}

	// https://discord.com/developers/docs/resources/webhook#edit-webhook-message
	var r discordgo.Message
	res, err := b.client.R().SetBody(attachments).SetResult(&r).Patch(b.webhookURL + "/messages/" + ts)
	if err != nil {
		return err
	}
	if res.StatusCode() >= 300 {
		return fmt.Errorf("failed to update previous message, %s", res.Body())
	}
	b.allocations[alloc.ID] = r.ID

	return nil
}

func (b *discordBot) initialAllocMsg(alloc api.Allocation) error {
	attachments := b.DefaultAttachmentsAlloc(alloc)
	if len(attachments.Embeds) == 0 {
		return nil
	}

	var r discordgo.Message
	_, err := b.client.R().SetBody(attachments).SetResult(&r).Post(b.webhookURL)
	if err != nil {
		return fmt.Errorf("post message failed,  err=%w", err)
	}
	b.deploys[alloc.ID] = r.ID
	return nil
}

func (b *discordBot) DefaultAttachmentsDeployment(deploy api.Deployment) discordgo.MessageSend {
	var content = bytes.NewBufferString("nomad deploy\n")
	content.WriteString(deploy.StatusDescription)
	content.WriteString("\n")

	var msg = discordgo.MessageSend{}

	var fields []*discordgo.MessageEmbed

	for tgn, tg := range deploy.TaskGroups {
		field := &discordgo.MessageEmbed{
			Color: discordColorForStatus(deploy.Status),
			Title: fmt.Sprintf("Task Group: %s", tgn),
			Description: fmt.Sprintf("Desired: %d, Placed: %d, Healthy: %d, Unhealthy: %d, DesiredCanaries: %d, PlacedCanaries: %+v",
				tg.DesiredTotal, tg.PlacedAllocs, tg.HealthyAllocs, tg.UnhealthyAllocs, tg.DesiredCanaries, tg.PlacedCanaries),
		}
		fields = append(fields, field)
	}
	msg.Embeds = fields

	fmt.Fprintf(content, "%s deployment update\n", deploy.JobID)
	fmt.Fprintf(content, "url: %s/ui/jobs/%s/deployments\n", b.nomadAddress, deploy.JobID)
	fmt.Fprintf(content, "Deploy ID: %s\n", deploy.ID)

	msg.Content = content.String()
	return msg
}

func (b *discordBot) DefaultAttachmentsAlloc(alloc api.Allocation) discordgo.MessageSend {
	var fields []*discordgo.MessageEmbed
	for taskName, taskState := range alloc.TaskStates {
		field := discordgo.MessageEmbed{
			Title: fmt.Sprintf("taskState:%s Failed: %v, Restarts: %d Task Group: %s Task: %s",
				taskState.State, taskState.Failed, taskState.Restarts, alloc.TaskGroup, taskName),
			Color: discordColorForStatus(alloc.ClientStatus),
		}
		gotOOM := false
		value := "---------------------------------------------\n"
		for _, event := range taskState.Events {
			if strings.Contains(event.DisplayMessage, "OOM") {
				gotOOM = true
			}

			value += fmt.Sprintf("*%s*: %s %s", event.Type, event.DisplayMessage, event.Details["driver_message"])
			if event.Type == structs.TaskTerminated {
				for _, key := range []string{"exit_code", "signal"} {
					if val, ok := event.Details[key]; ok && val != "" {
						value += fmt.Sprintf(", %s: %s", key, val)
					}
				}
			}
			if event.Type == structs.TaskKilled {
				for _, key := range []string{"kill_reason", "kill_error", "kill_timeout"} {
					if val, ok := event.Details[key]; ok && val != "" {
						value += fmt.Sprintf(", %s: %s", key, val)
					}
				}
			}
			value += "\n"
		}
		field.Description = value
		if !gotOOM {
			continue
		}
		fields = append(fields, &field)
	}

	if len(fields) == 0 {
		return discordgo.MessageSend{}
	}

	return discordgo.MessageSend{
		Content: fmt.Sprintf("Allocation ID: %s\n%s allocation update\nurl: %s/ui/jobs/%s/%s\n", alloc.ID, alloc.ID, b.nomadAddress, alloc.JobID, alloc.TaskGroup),
		Embeds:  fields,
	}
}

func discordColorForStatus(status string) int {
	switch status {
	case "failed":
		return 14503512 // "#dd4e58"
	case "running":
		return 1945343 // "#1daeff"
	case "successful":
		return 3581519 // "#36a64f"
	default:
		return 13882323 //"#D3D3D3"
	}
}
