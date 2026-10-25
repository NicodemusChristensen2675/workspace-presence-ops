package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/workspace-presence-ops/internal/infrai"
	"example.com/workspace-presence-ops/internal/workspace"
)

type realtimeAdapter struct{ client *infrai.Client }

func (a realtimeAdapter) Publish(ctx context.Context, in workspace.PublishInput) error {
	return a.client.Publish(ctx, infrai.PublishInput{
		Channel: in.Channel, Event: in.Event, Data: in.Data, AccountID: in.AccountID, RequestID: in.RequestID,
	})
}

func (a realtimeAdapter) Presence(ctx context.Context, channel string) (json.RawMessage, error) {
	return a.client.Presence(ctx, channel)
}

func main() {
	client, err := infrai.New(os.Getenv("INFRAI_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	reporter := workspace.NewReporter(realtimeAdapter{client: client})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /workspace/activity", activityHandler(reporter))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	server := &http.Server{Addr: envOr("ADDR", ":8080"), Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	log.Printf("workspace presence listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func activityHandler(reporter *workspace.Reporter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var activity workspace.Activity
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&activity); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid activity payload"})
			return
		}
		report, err := reporter.Record(r.Context(), activity)
		if err != nil {
			status := http.StatusBadGateway
			var apiErr *infrai.APIError
			if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
				status = apiErr.Status
			} else if errors.Is(err, workspace.ErrInvalidActivity) {
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
