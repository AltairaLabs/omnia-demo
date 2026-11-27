package signintest

import (
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fatalRecorder is a testing.TB whose Fatalf records the failure and ends
// the goroutine, so a test can check that the issuer refuses something.
type fatalRecorder struct {
	testing.TB
	mu  sync.Mutex
	msg string
}

func (f *fatalRecorder) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.msg = format
	f.mu.Unlock()
	runtime.Goexit()
}

func (f *fatalRecorder) Helper() {}

// approveFails reports whether Approve refuses authURL.
func approveFails(t *testing.T, authURL string) bool {
	t.Helper()
	id := NewIdentity(t)
	rec := &fatalRecorder{TB: t}
	iss := NewIssuer(t, id, "larkmere.example.test")
	iss.t = rec
	u := strings.Replace(authURL, "ISSUER", iss.URL, 1)
	u = strings.ReplaceAll(u, "CLIENT", url.QueryEscape(id.ClientID))
	u = strings.ReplaceAll(u, "REDIRECT", url.QueryEscape(id.RedirectURI))
	done := make(chan struct{})
	go func() {
		defer close(done)
		iss.Approve(u)
	}()
	<-done
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.msg != ""
}

func TestApproveChecksPromptAndResource(t *testing.T) {
	base := "ISSUER/authorize?response_type=code&client_id=CLIENT&redirect_uri=REDIRECT&scope=policies%3Aread&state=s&code_challenge=c&code_challenge_method=S256"
	if approveFails(t, base+"&prompt=consent") {
		t.Fatal("Approve refused a good authorization request")
	}
	if !approveFails(t, base) {
		t.Error("Approve accepted a request without prompt=consent")
	}
	if !approveFails(t, base+"&prompt=consent&resource=https%3A%2F%2Flarkmere.example.test") {
		t.Error("Approve accepted a request with a resource")
	}
}

func TestTokenRefusesReusedAssertion(t *testing.T) {
	id := NewIdentity(t)
	iss := NewIssuer(t, id, "larkmere.example.test")
	assertion, err := id.ClientAssertion(iss.URL, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"grant_type":            {"refresh_token"},
		"refresh_token":         {"r0"},
		"client_id":             {id.ClientID},
		"client_assertion_type": {assertionType},
		"client_assertion":      {assertion},
	}
	for i, want := range []int{http.StatusOK, http.StatusUnauthorized} {
		resp, err := iss.Client().PostForm(iss.URL+"/token", form)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("request %d: status %d, want %d", i+1, resp.StatusCode, want)
		}
	}
}
