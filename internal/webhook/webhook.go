// Package webhook decodes the raw payload contract published by ghook2pubsub.
package webhook

import (
	"bytes"
	"compress/gzip"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const MaxPayloadBytes = 25 << 20

// Event contains only fields used for routing and actions. Repository is always
// the top-level (base) repository, never a pull request's head repository.
type Event struct {
	Type              string
	Action            string
	RepositoryOwner   string
	RepositoryName    string
	RepositoryPrivate *bool
	Branch            string
	SHA               string
	WorkflowPath      string
	WorkflowEvent     string
	Conclusion        string
	RunID             int64
	RunAttempt        int
	Deleted           bool
	ContentID         string
}

// Decode accepts plain, gzip, and zstd data; payloads may be JSON or GitHub's
// form-urlencoded format. It never includes payload data in returned errors.
func Decode(data []byte, attrs map[string]string) (*Event, error) {
	if len(data) > MaxPayloadBytes {
		return nil, errors.New("webhook payload exceeds size limit")
	}
	var reader io.Reader = bytes.NewReader(data)
	switch attrs["compression"] {
	case "", "none":
	case "gzip":
		r, err := gzip.NewReader(reader)
		if err != nil {
			return nil, errors.New("invalid gzip payload")
		}
		defer r.Close()
		reader = r
	case "zstd":
		r, err := zstd.NewReader(reader, zstd.WithDecoderMaxMemory(64<<20))
		if err != nil {
			return nil, errors.New("invalid zstd payload")
		}
		defer r.Close()
		reader = r
	default:
		return nil, errors.New("unsupported payload compression")
	}
	body, err := io.ReadAll(io.LimitReader(reader, MaxPayloadBytes+1))
	if err != nil {
		return nil, errors.New("cannot decode payload")
	}
	if len(body) > MaxPayloadBytes {
		return nil, errors.New("decoded payload exceeds size limit")
	}
	if !jsontext.Value(body).IsValid() {
		form, err := url.ParseQuery(string(body))
		if err != nil || len(form["payload"]) != 1 {
			return nil, errors.New("invalid webhook payload")
		}
		body = []byte(form.Get("payload"))
	}
	var payload struct {
		Action      string `json:"action"`
		Ref         string `json:"ref"`
		After       string `json:"after"`
		Deleted     bool   `json:"deleted"`
		WorkflowRun *struct {
			ID             int64  `json:"id"`
			RunAttempt     int    `json:"run_attempt"`
			HeadBranch     string `json:"head_branch"`
			HeadSHA        string `json:"head_sha"`
			Path           string `json:"path"`
			Event          string `json:"event"`
			Conclusion     string `json:"conclusion"`
			HeadRepository *struct {
				Name  string `json:"name"`
				Owner struct {
					Login string `json:"login"`
				} `json:"owner"`
			} `json:"head_repository"`
		} `json:"workflow_run"`
		Repository *struct {
			Name    string `json:"name"`
			Private *bool  `json:"private"`
			Owner   struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
		Issue *struct {
			NodeID string `json:"node_id"`
		} `json:"issue"`
		PullRequest *struct {
			NodeID string `json:"node_id"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("invalid webhook JSON")
	}
	event := &Event{Type: attrs["gh_event"], Action: payload.Action}
	if event.Type != "issues" && event.Type != "pull_request" && event.Type != "workflow_run" && event.Type != "push" {
		return event, nil
	}
	if payload.Action == "" && event.Type != "push" {
		return nil, errors.New("missing webhook action")
	}
	if action := attrs["action"]; action != "" && action != payload.Action {
		return nil, errors.New("webhook action attribute mismatch")
	}
	if payload.Repository == nil || payload.Repository.Private == nil || payload.Repository.Name == "" || payload.Repository.Owner.Login == "" {
		return nil, errors.New("missing repository routing fields")
	}
	event.RepositoryOwner = payload.Repository.Owner.Login
	event.RepositoryName = payload.Repository.Name
	event.RepositoryPrivate = payload.Repository.Private
	if event.Type == "push" {
		event.Deleted = payload.Deleted
		if strings.HasPrefix(payload.Ref, "refs/heads/") {
			event.Branch = strings.TrimPrefix(payload.Ref, "refs/heads/")
		}
		event.SHA = payload.After
	}
	if event.Type == "workflow_run" {
		run := payload.WorkflowRun
		if run == nil {
			return nil, errors.New("missing workflow run")
		}
		// Fork runs never qualify as trusted source-repository completions.
		if run.HeadRepository != nil && strings.EqualFold(run.HeadRepository.Owner.Login, event.RepositoryOwner) && strings.EqualFold(run.HeadRepository.Name, event.RepositoryName) {
			event.Branch = run.HeadBranch
		}
		event.SHA, event.WorkflowPath, event.WorkflowEvent, event.Conclusion = run.HeadSHA, run.Path, run.Event, run.Conclusion
		event.RunID, event.RunAttempt = run.ID, run.RunAttempt
	}
	if event.Type == "issues" && payload.Issue != nil {
		event.ContentID = payload.Issue.NodeID
	}
	if event.Type == "pull_request" && payload.PullRequest != nil {
		event.ContentID = payload.PullRequest.NodeID
	}
	if (event.Type == "issues" || event.Type == "pull_request") && event.Action == "opened" && (event.ContentID == "" || len(event.ContentID) > 512 || strings.ContainsAny(event.ContentID, " \t\r\n")) {
		return nil, errors.New("missing or invalid content node ID")
	}
	return event, nil
}
