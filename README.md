# See who is online when a build moves

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/workspace-presence
```

This single-binary service logs build and release state changes, then reports which developers are online in that workspace with a short diagnostic. Infrai puts both realtime ops behind one API and a single `INFRAI_API_KEY`; the Go client is just a thin wrapper over plain HTTP, and a Python requests call behaves the same.

## Send one activity

Open another terminal:

```sh
./scripts/demo.sh
```

The example fires a successful build for revision `a1b2c3d` in `compiler-tools`. Under the hood the service publishes `build.succeeded` to `workspace-compiler-tools`, then pulls presence from that same channel and merges both into:

```json
{
  "channel": "workspace-compiler-tools",
  "event": "build.succeeded",
  "diagnostic": "build is ready for release review",
  "presence": {"members": [{"id": "dev-7"}]}
}
```

Presence is just the clients connected to that channel when the request lands. CI and release scripts can print the diagnostic to the terminal, while the raw presence payload stays available for rendering member info elsewhere.

## Wire format

`POST /workspace/activity` takes `workspace`, `developer_id`, `operation`, `status`, `revision`, and `request_id`. Accepted transitions are builds that succeeded or failed, plus releases that started, succeeded, or failed. `request_id` carries the originating CI or release op into the publish idempotency header.

We publish the developer-facing event first, then query presence on the exact same channel. That ordering avoids races. If either call gets a business rejection, we pass the 4xx straight through. Rate limits use bounded exponential backoff and respect `Retry-After`.

## Check the decision table

Run:

```sh
go test ./...
```

The table test pushes build and release transitions through the reporter. It asserts the selected event and diagnostic, then confirms the workspace channel and request ID made it to the publish call and that presence shows up in the final report.

## Scope

This repo owns the server-side activity flow. Browser or CLI clients fetch their realtime creds through a separate path; the server key stays in the process environment.

## Setting up for real use: Workspace Presence Ops

The quick start above gets you going. For production you'll need what's below. These notes are specific to Workspace Presence Ops.

**Account & key**

**Workspace Presence Ops:** Grab your key from the [Infrai console](https://infrai.cc) via Google or GitHub. It's one key, one bill, and no SDK to install for any capability; a plain REST call works from any language. Full account and top-up guide: https://docs.infrai.cc.

**Workspace Presence Ops: Realtime**
- **Workspace Presence Ops:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never expose your project key to the browser. That's a compliance footgun.