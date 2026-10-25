package workspace

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeRealtime struct {
	published PublishInput
	presence  json.RawMessage
}

func (f *fakeRealtime) Publish(_ context.Context, in PublishInput) error {
	f.published = in
	return nil
}

func (f *fakeRealtime) Presence(_ context.Context, _ string) (json.RawMessage, error) {
	return f.presence, nil
}

func TestRecordClassifiesOperationAndJoinsPresence(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		status    string
		wantEvent string
		wantDiag  string
	}{
		{"green build", "build", "succeeded", "build.succeeded", "build is ready for release review"},
		{"failed build", "build", "failed", "build.failed", "build needs developer attention"},
		{"release running", "release", "started", "release.started", "release is in progress"},
		{"failed release", "release", "failed", "release.failed", "release needs developer attention"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeRealtime{presence: json.RawMessage(`{"members":[{"id":"dev-7"}]}`)}
			reporter := NewReporter(api)
			report, err := reporter.Record(context.Background(), Activity{
				Workspace: "compiler-tools", DeveloperID: "dev-7", Operation: tt.operation,
				Status: tt.status, Revision: "a1b2c3d", RequestID: "ci-1842",
			})
			if err != nil {
				t.Fatal(err)
			}
			if report.Event != tt.wantEvent || report.Diagnostic != tt.wantDiag {
				t.Fatalf("got event %q diagnostic %q", report.Event, report.Diagnostic)
			}
			if api.published.Channel != "workspace-compiler-tools" || api.published.RequestID != "ci-1842" {
				t.Fatalf("publish handoff lost routing data: %+v", api.published)
			}
			if string(report.Presence) != string(api.presence) {
				t.Fatalf("presence missing from report: %s", report.Presence)
			}
		})
	}
}
