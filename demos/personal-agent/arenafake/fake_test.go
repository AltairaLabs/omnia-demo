package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type client struct {
	t   *testing.T
	url string
}

func newClient(t *testing.T) *client {
	srv := httptest.NewServer(newFake().handler())
	t.Cleanup(srv.Close)
	return &client{t: t, url: srv.URL}
}

func (c *client) call(tool string, args map[string]string) (int, map[string]any) {
	c.t.Helper()
	b, _ := json.Marshal(args)
	resp, err := http.Post(c.url+"/tools/"+tool, "application/json", bytes.NewReader(b))
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		c.t.Fatal(err)
	}
	return resp.StatusCode, out
}

func (c *client) ok(tool string, args map[string]string) map[string]any {
	c.t.Helper()
	status, out := c.call(tool, args)
	if status != http.StatusOK {
		c.t.Fatalf("%s: status %d: %v", tool, status, out)
	}
	return out
}

// last is the newest company message and the sign_in block of a read.
func (c *client) last(id string) (string, map[string]any) {
	c.t.Helper()
	out := c.ok("read_conversation", map[string]string{"conversation_id": id})
	msgs, _ := out["messages"].([]any)
	text := ""
	for _, m := range msgs {
		mm := m.(map[string]any)
		if mm["role"] == "company" {
			text = mm["text"].(string)
		}
	}
	si, _ := out["sign_in"].(map[string]any)
	return text, si
}

func TestStormStory(t *testing.T) {
	c := newClient(t)
	w := c.ok("start_conversation", map[string]string{"domain": "@WRENMOOR_DOMAIN@",
		"message": "Ellen's power is off at HX7 6QA after the storm. Please check her supply."})
	l := c.ok("start_conversation", map[string]string{"domain": "@LARKMERE_DOMAIN@",
		"message": "Water has come in at Ellen's home in the storm. Please report a claim."})
	wid, lid := w["conversation_id"].(string), l["conversation_id"].(string)
	if wid != "conv-wrenmoor-1" || lid != "conv-larkmere-1" {
		t.Fatalf("ids %q %q", wid, lid)
	}

	text, si := c.last(wid)
	if !strings.Contains(text, "18:00") || si["needed"] != true || si["scopes"] != "outages" {
		t.Fatalf("wrenmoor first reply %q %v", text, si)
	}
	text, si = c.last(lid)
	if !strings.Contains(text, "signed in") || si["scopes"] != "claims" || si["status"] != "needed" {
		t.Fatalf("larkmere first reply %q %v", text, si)
	}

	got := c.ok("sign_in", map[string]string{"domain": "@LARKMERE_DOMAIN@", "scopes": "claims"})
	if got["status"] != "pending" || !strings.Contains(got["note"].(string), "button") {
		t.Fatalf("sign_in %v", got)
	}
	if _, si = c.last(lid); si["status"] != "pending" {
		t.Fatalf("after sign_in %v", si)
	}
	c.ok("send_message", map[string]string{"conversation_id": lid, "text": "Please go ahead and report the claim."})
	text, si = c.last(lid)
	if !strings.Contains(text, "LM-41027") || si["status"] != "signed_in" || si["needed"] != false {
		t.Fatalf("larkmere after sign-in %q %v", text, si)
	}
	c.ok("send_message", map[string]string{"conversation_id": lid, "text": "Ellen does not accept the offer yet."})
	if text, _ = c.last(lid); text != larkNotSettled {
		t.Fatalf("not settled: %q", text)
	}

	c.ok("sign_in", map[string]string{"domain": "@WRENMOOR_DOMAIN@", "scopes": "outages"})
	c.ok("send_message", map[string]string{"conversation_id": wid, "text": "Ellen would rather not sign in now."})
	if text, si = c.last(wid); text != wrenDeclined || si["status"] != "declined" {
		t.Fatalf("declined: %q %v", text, si)
	}
}

func TestScriptsByTopic(t *testing.T) {
	for _, tc := range []struct{ domain, message, want string }{
		{"@WRENMOOR_DOMAIN@", "Is any planned power cut due in HX7 this week?", wrenPlanned},
		{"@WRENMOOR_DOMAIN@", "Is Ellen owed compensation for last month's power cut?", wrenLastMonth},
		{"@WRENMOOR_DOMAIN@", "Is Ellen's power off?", wrenAskPostcode},
		{"@LARKMERE_DOMAIN@", "How is Ellen's fence claim going?", larkFence},
	} {
		c := newClient(t)
		out := c.ok("start_conversation", map[string]string{"domain": tc.domain, "message": tc.message})
		if text, _ := c.last(out["conversation_id"].(string)); text != tc.want {
			t.Errorf("%q: got %q", tc.message, text)
		}
	}
}

func TestPostcodeThenOutage(t *testing.T) {
	c := newClient(t)
	c.ok("start_conversation", map[string]string{"domain": "wrenmoor.example.com", "message": "Is Ellen's power off?"})
	c.ok("send_message", map[string]string{"conversation_id": "conv-wrenmoor-1", "text": "Her postcode is HX7 6QA."})
	if text, si := c.last("conv-wrenmoor-1"); text != wrenOutage || si["needed"] != true {
		t.Fatalf("got %q %v", text, si)
	}
}

func TestFenceOfferAcceptance(t *testing.T) {
	for text, want := range map[string]string{
		"Ellen accepts the £390.":                 larkFenceAccepted,
		"Ellen doesn't want to accept the offer.": larkFenceDeclined,
		"She'll think about it.":                  larkFenceDeclined,
	} {
		c := newClient(t)
		c.ok("start_conversation", map[string]string{"domain": "@LARKMERE_DOMAIN@", "message": "How is her fence claim going?"})
		c.ok("send_message", map[string]string{"conversation_id": "conv-larkmere-1", "text": text})
		if got, _ := c.last("conv-larkmere-1"); got != want {
			t.Errorf("%q: got %q", text, got)
		}
	}
}

func TestUnknownCompanyNeedsFindCompany(t *testing.T) {
	c := newClient(t)
	if status, _ := c.call("start_conversation", map[string]string{"domain": "halvern.example.com", "message": "hi"}); status != http.StatusBadRequest {
		t.Fatalf("start before find: status %d", status)
	}
	if out := c.ok("find_company", map[string]string{"domain": "nobody.example.com"}); out["found"] != false {
		t.Fatalf("unknown found: %v", out)
	}
	if out := c.ok("find_company", map[string]string{"domain": "halvern.example.com"}); out["name"] != "Halvern Partners" {
		t.Fatalf("halvern: %v", out)
	}
	c.ok("start_conversation", map[string]string{"domain": "halvern.example.com", "message": "Ellen needs a roofer."})
	if status, _ := c.call("read_conversation", map[string]string{"conversation_id": "conv-made-up"}); status != http.StatusNotFound {
		t.Fatalf("made-up id: status %d", status)
	}
}

// TestToolsMatchShipped keeps the kit's test tools (arena/test/tools) the
// shipped tools (arena/tools) in all but their binding: the model must see
// exactly what it sees in the pack. To regenerate a test tool after changing
// the shipped one, from demos/personal-agent/arena:
//
//	yq '.spec.mode = "live" | del(.spec.mock_result) | .spec.http = {...}' tools/x.tool.yaml
//
// keeping the head comment and http block already in test/tools/x.tool.yaml.
func TestToolsMatchShipped(t *testing.T) {
	shipped, _ := filepath.Glob("../arena/tools/*.tool.yaml")
	if len(shipped) == 0 {
		t.Fatal("no shipped tools found")
	}
	for _, path := range shipped {
		name := filepath.Base(path)
		want, got := toolSpec(t, path), toolSpec(t, filepath.Join("../arena/test/tools", name))
		for _, k := range []string{"name", "description", "input_schema", "output_schema", "action_scope", "timeout_ms"} {
			if !reflect.DeepEqual(want[k], got[k]) {
				t.Errorf("%s: spec.%s differs from the shipped tool", name, k)
			}
		}
		if got["mode"] != "live" || got["http"] == nil {
			t.Errorf("%s: the test tool is not bound to arenafake", name)
		}
	}
}

func toolSpec(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Spec map[string]any `yaml:"spec"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc.Spec
}
