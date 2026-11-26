package azure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// omniaValuesRun runs omnia-values.sh against a site fixture, writing to a
// temp file. It returns the script output, the written file and yq's answers.
func omniaValuesRun(t *testing.T, site string) (out, file string) {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "o", "omnia-values.yaml")
	script := `export SITE="$PWD/testdata/${SITEFILE}"; export OUT="` + dest + `"; bash ./omnia-values.sh
`
	_, out, err := runScript(t, script, siteEnv(site), nil)
	if err != nil {
		t.Fatal(err, out)
	}
	b, rErr := os.ReadFile(dest)
	if rErr != nil {
		t.Fatal(rErr)
	}
	return out, string(b)
}

func yqRead(t *testing.T, doc, expr string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "v.yaml")
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, out, err := runScript(t, `yq e '`+expr+`' '`+p+`'`, nil, nil)
	if err != nil {
		t.Fatal(err, out)
	}
	return strings.TrimSpace(out)
}

var wantIssuers = []string{
	"http://larkmere-portal.larkmere-mutual.svc.cluster.local:8080",
	"http://card-callers.larkmere-mutual.svc.cluster.local:8080",
	"http://wrenmoor-site.wrenmoor-energy.svc.cluster.local:8080",
	"http://assistant-page.ellens-assistant.svc.cluster.local:8080",
}

func TestOmniaValuesUmbrellaRoot(t *testing.T) {
	_, doc := omniaValuesRun(t, "site-omnia-umbrella.yaml")
	if got := yqRead(t, doc, ".omnia.operator.delegationAPI.trustedIssuers | length"); got != "4" {
		t.Fatalf("want 4 issuers under omnia, got %s\n%s", got, doc)
	}
	if got := yqRead(t, doc, ".operator"); got != "null" {
		t.Errorf("nothing belongs at top level, got %s", got)
	}
	if got := yqRead(t, doc, ".omnia.operator.defaultExposure | [.enabled, .baseDomain, .gateway.name, .gateway.namespace, .gateway.sectionName] | join(\",\")"); got != "true,demo.example.com,omnia-external,omnia,https" {
		t.Errorf("defaultExposure: %s", got)
	}
}

func TestOmniaValuesBareRoot(t *testing.T) {
	_, doc := omniaValuesRun(t, "site-omnia-bare.yaml")
	if got := yqRead(t, doc, ".operator.delegationAPI.trustedIssuers | length"); got != "4" {
		t.Fatalf("want 4 issuers at top level, got %s\n%s", got, doc)
	}
	if got := yqRead(t, doc, ".omnia"); got != "null" {
		t.Errorf("no omnia key expected, got %s", got)
	}
	if got := yqRead(t, doc, ".operator.defaultExposure.baseDomain"); got != "demo.example.com" {
		t.Errorf("baseDomain: %s", got)
	}
}

func TestOmniaValuesIssuersExact(t *testing.T) {
	_, doc := omniaValuesRun(t, "site-omnia-bare.yaml")
	for i, iss := range wantIssuers {
		want := iss + "=" + iss + "/.well-known/jwks.json"
		if got := yqRead(t, doc, ".operator.delegationAPI.trustedIssuers["+string(rune('0'+i))+"]"); got != want {
			t.Errorf("issuer %d: got %q want %q", i, got, want)
		}
	}
	if !strings.Contains(doc, "stand-in") || !strings.Contains(doc, "make omnia-values") {
		t.Errorf("header and stand-in comments missing:\n%s", doc)
	}
}

func TestOmniaValuesTLSHosts(t *testing.T) {
	out, _ := omniaValuesRun(t, "site-omnia-umbrella.yaml")
	want := "TLS hosts: larkmere.demo.example.com wrenmoor.demo.example.com auth.larkmere.demo.example.com auth.wrenmoor.demo.example.com assistant.demo.example.com dashboard.demo.example.com"
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "TLS hosts:") {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 || lines[0] != want {
		t.Fatalf("want exactly %q, got %q", want, lines)
	}
}

func TestOmniaValuesMissingGatewayFails(t *testing.T) {
	_, out, err := runScript(t, `export SITE="$PWD/testdata/site-min.yaml"; export OUT="$(mktemp -d)/x.yaml"; bash ./omnia-values.sh`, nil, nil)
	if err == nil || !strings.Contains(out, "is required") {
		t.Fatalf("want a required-field failure, got %v %s", err, out)
	}
}
