package bot

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/slack-go/slack"
)

type Config struct {
	// You more than likely want your "Bot User OAuth Access Token" which starts with "xoxb-"
	Token   string
	Channel string
}

type Bot struct {
	mu           sync.Mutex
	chanID       string
	nomadAddress string
	api          *slack.Client
	deploys      map[string]string
	allocations  map[string]string
	L            hclog.Logger
}

func NewBot(cfg Config, nomadAddress string) (*Bot, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("no token provided")
	}

	api := slack.New(cfg.Token)

	bot := &Bot{
		api:          api,
		nomadAddress: nomadAddress,
		chanID:       cfg.Channel,
		deploys:      make(map[string]string),
		allocations:  make(map[string]string),
		L:            hclog.Default(),
	}

	return bot, nil
}

func (b *Bot) UpsertDeployMsg(deploy api.Deployment) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ts, ok := b.deploys[deploy.ID]
	if !ok {
		return b.initialDeployMsg(deploy)
	}
	b.L.Debug("Existing deployment found, updating status", "slack ts", ts)

	attachments := b.DefaultAttachmentsDeployment(deploy)
	opts := []slack.MsgOption{slack.MsgOptionAttachments(attachments...)}
	opts = append(opts, DefaultDeployMsgOpts()...)

	_, ts, _, err := b.api.UpdateMessage(b.chanID, ts, opts...)
	if err != nil {
		return err
	}
	b.deploys[deploy.ID] = ts

	return nil
}

func (b *Bot) initialDeployMsg(deploy api.Deployment) error {
	attachments := b.DefaultAttachmentsDeployment(deploy)

	opts := []slack.MsgOption{slack.MsgOptionAttachments(attachments...)}
	opts = append(opts, DefaultDeployMsgOpts()...)

	_, ts, err := b.api.PostMessage(b.chanID, opts...)
	if err != nil {
		return fmt.Errorf("post message failed,  err=%w", err)
	}
	b.deploys[deploy.ID] = ts
	return nil
}

func (b *Bot) UpsertAllocationMsg(alloc api.Allocation) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ts, ok := b.allocations[alloc.ID]
	if !ok {
		return b.initialAllocMsg(alloc)
	}
	b.L.Debug("Existing allocation found, updating status", "slack ts", ts)

	attachments := b.DefaultAttachmentsAlloc(alloc)
	if len(attachments) == 0 {
		return nil
	}

	opts := []slack.MsgOption{slack.MsgOptionAttachments(attachments...)}
	opts = append(opts, DefaultDeployMsgOpts()...)

	_, ts, _, err := b.api.UpdateMessage(b.chanID, ts, opts...)
	if err != nil {
		return err
	}
	b.allocations[alloc.ID] = ts

	return nil
}

func (b *Bot) initialAllocMsg(alloc api.Allocation) error {
	attachments := b.DefaultAttachmentsAlloc(alloc)
	if len(attachments) == 0 {
		return nil
	}

	opts := []slack.MsgOption{slack.MsgOptionAttachments(attachments...)}
	opts = append(opts, DefaultDeployMsgOpts()...)

	_, ts, err := b.api.PostMessage(b.chanID, opts...)
	if err != nil {
		return fmt.Errorf("post message failed,  err=%w", err)
	}
	b.deploys[alloc.ID] = ts
	return nil
}

func DefaultDeployMsgOpts() []slack.MsgOption {
	return []slack.MsgOption{
		slack.MsgOptionAsUser(true),
	}
}

func (b *Bot) DefaultAttachmentsDeployment(deploy api.Deployment) []slack.Attachment {
	var actions []slack.AttachmentAction
	if deploy.StatusDescription == "Deployment is running but requires manual promotion" {
		actions = []slack.AttachmentAction{
			{
				Name: "promote",
				Text: "Promote :heavy_check_mark:",
				Type: "button",
			},
			{
				Name:  "fail",
				Text:  "Fail :boom:",
				Style: "danger",
				Type:  "button",
				Confirm: &slack.ConfirmationField{
					Title:       "Are you sure?",
					Text:        ":nomad-sad: :nomad-sad: :nomad-sad: :nomad-sad: :nomad-sad:",
					OkText:      "Fail",
					DismissText: "Woops!",
				},
			},
		}
	}
	var fields []slack.AttachmentField
	for tgn, tg := range deploy.TaskGroups {
		field := slack.AttachmentField{
			Title: fmt.Sprintf("Task Group: %s", tgn),
			Value: fmt.Sprintf("Healthy: %d, Placed: %d, Desired Canaries: %d", tg.HealthyAllocs, tg.PlacedAllocs, tg.DesiredCanaries),
		}
		fields = append(fields, field)
	}
	return []slack.Attachment{
		{
			Fallback:   "deployment update",
			Color:      colorForStatus(deploy.Status),
			AuthorName: fmt.Sprintf("%s deployment update", deploy.JobID),
			AuthorLink: fmt.Sprintf("%s/ui/jobs/%s/deployments", b.nomadAddress, deploy.JobID),
			Title:      deploy.StatusDescription,
			TitleLink:  fmt.Sprintf("%s/ui/jobs/%s/deployments", b.nomadAddress, deploy.JobID),
			Fields:     fields,
			Footer:     fmt.Sprintf("Deploy ID: %s", deploy.ID),
			Ts:         json.Number(fmt.Sprintf("%d", time.Now().Unix())),
			Actions:    actions,
		},
	}
}

func (b *Bot) DefaultAttachmentsAlloc(alloc api.Allocation) []slack.Attachment {

	var actions []slack.AttachmentAction
	var fields []slack.AttachmentField
	for taskName, taskState := range alloc.TaskStates {
		field := slack.AttachmentField{
			Title: fmt.Sprintf("taskState:%s Failed: %v, Restarts: %d Task Group: %s Task: %s",
				taskState.State, taskState.Failed, taskState.Restarts, alloc.TaskGroup, taskName),
			Value: "",
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
		field.Value = value
		if !gotOOM {
			continue
		}
		fields = append(fields, field)
	}

	if len(fields) == 0 {
		return []slack.Attachment{}
	}
	return []slack.Attachment{
		{
			Fallback:   "allocation update",
			Color:      colorForStatus(alloc.ClientStatus),
			AuthorName: fmt.Sprintf("%s allocation update", alloc.ID),
			AuthorLink: fmt.Sprintf("%s/ui/allocations/%s", b.nomadAddress, alloc.ID),
			Title:      alloc.ClientDescription,
			TitleLink:  fmt.Sprintf("%s/ui/jobs/%s/%s", b.nomadAddress, alloc.JobID, alloc.TaskGroup),
			Fields:     fields,
			Footer:     fmt.Sprintf("Allocation ID: %s", alloc.ID),
			Ts:         json.Number(fmt.Sprintf("%d", time.Now().Unix())),
			Actions:    actions,
		},
	}
}

func colorForStatus(status string) string {
	switch status {
	case "failed":
		return "#dd4e58"
	case "running":
		return "#1daeff"
	case "successful":
		return "#36a64f"
	default:
		return "#D3D3D3"
	}
}
