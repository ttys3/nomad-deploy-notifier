#!/usr/bin/env bash

set -eou pipefail

env SLACK_TOKEN=xoxb-xxxxxx SLACK_CHANNEL=CXXXXXXXX ./nomad-event-notifier

