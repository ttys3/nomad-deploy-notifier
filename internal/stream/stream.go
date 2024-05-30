package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/api"

	"github.com/ttys3/nomad-event-notifier/internal/bot"
)

type Stream struct {
	nomad *api.Client
	L     hclog.Logger
}

func NewStream(config *api.Config) (*Stream, error) {
	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating nomad client: %w", err)
	}
	return &Stream{
		nomad: client,
		L:     hclog.Default(),
	}, nil
}

// https://www.nomadproject.io/api-docs/events
func (s *Stream) Subscribe(ctx context.Context, b *bot.Bot) {
	events := s.nomad.EventStream()

	// Topic: Node, Job, Evaluation, Allocation, Deployment
	// event topic: job
	topics := map[api.Topic][]string{
		api.Topic("Deployment"): {"*"},
		api.Topic("Allocation"): {"*"},
		// api.Topic("Job"):        {"*"},
	}

	// index (int: 0) - Specifies the index to start streaming events from.
	// If the requested index is no longer in the buffer the stream will start at the next available index.
	// hack: use math.MaxInt64 to avoid duplicated items each time server restart
	eventCh, err := events.Stream(ctx, topics, math.MaxInt64, &api.QueryOptions{})
	if err != nil {
		s.L.Error("error creating event stream client", "error", err)
		os.Exit(1)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-eventCh:
			if event.Err != nil {
				s.L.Warn("error from event stream", "error", event.Err)
				break
			}
			if event.IsHeartbeat() {
				s.L.Info("got heartbeat")
				continue
			}

			// Topic: Node, Job, Evaluation, Allocation, Deployment
			for _, e := range event.Events {
				eventJson, _ := json.Marshal(e)
				s.L.Info("got event", "topic", e.Topic, "evt_type", e.Type, "event", string(eventJson))

				switch e.Topic {
				case "Allocation":
					// PlanResult, AllocationUpdated, AllocationUpdateDesiredStatus
					alloc, err := e.Allocation()
					if err != nil {
						s.L.Error("decode Payload as Allocation failed", "error", err)
						continue
					}

					if alloc != nil {
						allocJson, _ := json.Marshal(alloc)
						s.L.Info("got Allocation", "allocation", string(allocJson))
						if err = b.UpsertAllocationMsg(*alloc); err != nil {
							s.L.Warn("error UpsertAllocationMsg", "error", err)
							continue
						}
					}
				case "Deployment":
					deployment, err := e.Deployment()
					if err != nil {
						s.L.Error("decode Payload as Deployment failed", "error", err)
						continue
					}
					if deployment == nil {
						s.L.Error("nil deployment")
						continue
					}

					deploymentJson, _ := json.Marshal(deployment)
					s.L.Info("got Deployment", "deployment", string(deploymentJson))
					if err = b.UpsertDeployMsg(*deployment); err != nil {
						s.L.Warn("error UpsertDeployMsg", "error", err)
						continue
					}
				}
			}
		default:
			time.Sleep(time.Millisecond * 100)
		} // end select
	} // end for
}
