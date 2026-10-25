package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

var workspaceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

var ErrInvalidActivity = errors.New("invalid activity")

type Realtime interface {
	Publish(context.Context, PublishInput) error
	Presence(context.Context, string) (json.RawMessage, error)
}

type PublishInput struct {
	Channel   string
	Event     string
	Data      any
	AccountID string
	RequestID string
}

type Activity struct {
	Workspace   string `json:"workspace"`
	DeveloperID string `json:"developer_id"`
	Operation   string `json:"operation"`
	Status      string `json:"status"`
	Revision    string `json:"revision"`
	RequestID   string `json:"request_id"`
}

type Report struct {
	Channel    string          `json:"channel"`
	Event      string          `json:"event"`
	Diagnostic string          `json:"diagnostic"`
	Presence   json.RawMessage `json:"presence"`
}

type Reporter struct{ realtime Realtime }

func NewReporter(realtime Realtime) *Reporter { return &Reporter{realtime: realtime} }

func (r *Reporter) Record(ctx context.Context, activity Activity) (Report, error) {
	event, diagnostic, err := classify(activity)
	if err != nil {
		return Report{}, err
	}
	channel := "workspace-" + activity.Workspace
	publish := PublishInput{
		Channel: channel,
		Event:   event,
		Data: map[string]string{
			"operation": activity.Operation,
			"status":    activity.Status,
			"revision":  activity.Revision,
		},
		AccountID: activity.DeveloperID,
		RequestID: activity.RequestID,
	}
	if err := r.realtime.Publish(ctx, publish); err != nil {
		return Report{}, fmt.Errorf("publish activity: %w", err)
	}
	presence, err := r.realtime.Presence(ctx, channel)
	if err != nil {
		return Report{}, fmt.Errorf("read workspace presence: %w", err)
	}
	return Report{Channel: channel, Event: event, Diagnostic: diagnostic, Presence: presence}, nil
}

func classify(activity Activity) (string, string, error) {
	if !workspaceName.MatchString(activity.Workspace) || activity.DeveloperID == "" || activity.Revision == "" || activity.RequestID == "" {
		return "", "", fmt.Errorf("%w: workspace, developer_id, revision, and request_id are required", ErrInvalidActivity)
	}
	switch activity.Operation + ":" + activity.Status {
	case "build:succeeded":
		return "build.succeeded", "build is ready for release review", nil
	case "build:failed":
		return "build.failed", "build needs developer attention", nil
	case "release:started":
		return "release.started", "release is in progress", nil
	case "release:succeeded":
		return "release.succeeded", "release completed", nil
	case "release:failed":
		return "release.failed", "release needs developer attention", nil
	default:
		return "", "", fmt.Errorf("%w: operation and status must describe a build or release transition", ErrInvalidActivity)
	}
}
