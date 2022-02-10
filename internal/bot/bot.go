package bot

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/api"
	"github.com/slack-go/slack"
)

type Config struct {
	// You more than likely want your "Bot User OAuth Access Token" which starts with "xoxb-"
	Token   string
	Channel string
}

type Bot struct {
	mu          sync.Mutex
	chanID      string
	api         *slack.Client
	deploys     map[string]string
	allocations map[string]string
	L           hclog.Logger
}

func NewBot(cfg Config) (*Bot, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("no token provided")
	}

	api := slack.New(cfg.Token)

	bot := &Bot{
		api:         api,
		chanID:      cfg.Channel,
		deploys:     make(map[string]string),
		allocations: make(map[string]string),
		L:           hclog.Default(),
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

	attachments := DefaultAttachmentsDeployment(deploy)
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
	attachments := DefaultAttachmentsDeployment(deploy)

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

	attachments := DefaultAttachmentsAlloc(alloc)
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
	attachments := DefaultAttachmentsAlloc(alloc)

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

func DefaultAttachmentsDeployment(deploy api.Deployment) []slack.Attachment {
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
			AuthorLink: fmt.Sprintf("http://127.0.0.1:4646/ui/jobs/%s/deployments", deploy.JobID),
			Title:      deploy.StatusDescription,
			TitleLink:  fmt.Sprintf("http://127.0.0.1:4646/ui/jobs/%s/deployments", deploy.JobID),
			Fields:     fields,
			Footer:     fmt.Sprintf("Deploy ID: %s", deploy.ID),
			Ts:         json.Number(fmt.Sprintf("%d", time.Now().Unix())),
			Actions:    actions,
		},
	}
}

func DefaultAttachmentsAlloc(alloc api.Allocation) []slack.Attachment {
	var actions []slack.AttachmentAction
	var fields []slack.AttachmentField
	for taskName, taskState := range alloc.TaskStates {
		for _, event := range taskState.Events {
			field := slack.AttachmentField{
				Title: fmt.Sprintf("Task Group: %s Allocation: %s Task: %s", alloc.TaskGroup, alloc.ID, taskName),
				Value: fmt.Sprintf("DisplayMessage: %s, ExitCode: %d, KillReason: %s KillError: %s",
					event.DisplayMessage, event.ExitCode, event.KillReason, event.KillError),
			}
			fields = append(fields, field)
		}
	}
	return []slack.Attachment{
		{
			Fallback:   "allocation update",
			Color:      colorForStatus(alloc.ClientStatus),
			AuthorName: fmt.Sprintf("%s allocation update", alloc.ID),
			AuthorLink: fmt.Sprintf("http://127.0.0.1:4646/ui/allocations/%s", alloc.ID),
			Title:      alloc.ClientDescription,
			TitleLink:  fmt.Sprintf("http://127.0.0.1:4646/ui/jobs/%s/%s", alloc.JobID, alloc.TaskGroup),
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
