package webhook

import (
	"bytes"
	"compress/gzip"
	"encoding/json/v2"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

const issue = `{"action":"opened","repository":{"owner":{"login":"Org"},"name":"repo","private":false},"issue":{"node_id":"I_123"}}`

func TestDecodeTransports(t *testing.T) {
	for _, format := range []string{"json", "form", "gzip", "zstd"} {
		t.Run(format, func(t *testing.T) {
			data := []byte(issue)
			attrs := map[string]string{"gh_event": "issues", "action": "opened"}
			switch format {
			case "form":
				data = []byte(url.Values{"payload": {issue}}.Encode())
			case "gzip":
				var b bytes.Buffer
				w := gzip.NewWriter(&b)
				w.Write(data)
				w.Close()
				data = b.Bytes()
				attrs["compression"] = "gzip"
			case "zstd":
				w, _ := zstd.NewWriter(nil)
				data = w.EncodeAll(data, nil)
				w.Close()
				attrs["compression"] = "zstd"
			}
			event, err := Decode(data, attrs)
			if err != nil {
				t.Fatal(err)
			}
			if event.ContentID != "I_123" || event.RepositoryPrivate == nil || *event.RepositoryPrivate || event.RepositoryOwner != "Org" {
				t.Fatalf("bad event: %+v", event)
			}
		})
	}
}
func TestRequiresPrivacyAndBaseRepository(t *testing.T) {
	var p map[string]any
	json.Unmarshal([]byte(issue), &p)
	delete(p["repository"].(map[string]any), "private")
	data, _ := json.Marshal(p)
	if _, err := Decode(data, map[string]string{"gh_event": "issues"}); err == nil {
		t.Fatal("missing privacy accepted")
	}
	data = []byte(`{"action":"opened","repository":{"owner":{"login":"Base"},"name":"repo","private":true},"pull_request":{"node_id":"PR_1","head":{"repo":{"private":false,"owner":{"login":"Fork"}}}}}`)
	e, err := Decode(data, map[string]string{"gh_event": "pull_request"})
	if err != nil || e.RepositoryOwner != "Base" || !*e.RepositoryPrivate || e.ContentID != "PR_1" {
		t.Fatalf("wrong repository: %+v %v", e, err)
	}
}
func TestRejectInvalidAndBounded(t *testing.T) {
	for _, tc := range []struct {
		data  []byte
		attrs map[string]string
	}{{[]byte(issue), map[string]string{"gh_event": "issues", "action": "closed"}}, {[]byte("nope"), nil}, {[]byte(issue), map[string]string{"compression": "brotli"}}, {[]byte("nope"), map[string]string{"compression": "gzip"}}} {
		if _, err := Decode(tc.data, tc.attrs); err == nil {
			t.Fatal("invalid accepted")
		}
	}
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(bytes.Repeat([]byte("x"), MaxPayloadBytes+1))
	w.Close()
	if _, err := Decode(b.Bytes(), map[string]string{"compression": "gzip"}); err == nil {
		t.Fatal("oversize decompression accepted")
	}
}

func TestWorkflowSourceRepository(t *testing.T) {
	for _, tt := range []struct {
		head string
		want string
	}{
		{`{"name":"repo","owner":{"login":"Org"}}`, "main"},
		{`{"name":"repo","owner":{"login":"fork-owner"}}`, ""},
		{`null`, ""},
	} {
		payload := `{"action":"completed","repository":{"name":"repo","private":false,"owner":{"login":"Org"}},"workflow_run":{"head_branch":"main","head_repository":` + tt.head + `}}`
		e, err := Decode([]byte(payload), map[string]string{"gh_event": "workflow_run", "action": "completed"})
		if err != nil || e.Branch != tt.want {
			t.Fatalf("fork routing: %+v %v", e, err)
		}
	}
}

func TestRejectAmbiguousJSON(t *testing.T) {
	cases := map[string]string{
		"duplicate routing field": strings.Replace(issue, `"private":false`, `"private":false,"private":true`, 1),
		"invalid UTF-8":           strings.Replace(issue, `"Org"`, "\"\xff\"", 1),
		"case-sensitive field":    strings.Replace(issue, `"private":false`, `"Private":false`, 1),
	}
	for name, payload := range cases {
		for _, form := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/form=%t", name, form), func(t *testing.T) {
				if form {
					payload = url.Values{"payload": {payload}}.Encode()
				}
				if _, err := Decode([]byte(payload), map[string]string{"gh_event": "issues"}); err == nil {
					t.Fatal("ambiguous payload accepted")
				}
			})
		}
	}
}
