package azure

import (
	"strings"
	"testing"
)

const tenantScript = `export SITE="$PWD/testdata/${SITEFILE}"; bash ./tenant.sh ${TENANT}
`

func tenantEnv(site string, extra map[string]string) map[string]string {
	env := map[string]string{"SITEFILE": site, "TENANT": "larkmere-mutual"}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// A tenant whose identity, grants and cluster state are all new (the identity
// lookup fails; create answers with its ids).
var tenantNewFixtures = map[string]string{
	"*identity_create*":       `{"clientId":"cid-1","principalId":"pid-1"}`,
	"*account_show*":          "tenant-1",
	"*get_daemonset_ztunnel*": "ztunnel",
}

// The same tenant, already set up.
var tenantExistingFixtures = map[string]string{
	"*identity_show*":         `{"clientId":"cid-1","principalId":"pid-1"}`,
	"*account_show*":          "tenant-1",
	"*role_assignment_list*":  "/assignment/1",
	"*get_daemonset_ztunnel*": "ztunnel",
}

func count(log []string, parts ...string) int {
	n := 0
	for _, l := range log {
		ok := true
		for _, p := range parts {
			if !strings.Contains(l, p) {
				ok = false
			}
		}
		if ok {
			n++
		}
	}
	return n
}

func TestTenantCreatesIdentityAndGrants(t *testing.T) {
	r := runScriptFull(t, tenantScript, tenantEnv("site-full.yaml", map[string]string{
		"FAKE_FAIL": "az * identity show*",
	}), tenantNewFixtures)
	if r.Err != nil {
		t.Fatal(r.Err, r.Out)
	}
	assertContexts(t, r.Log, testCtx, testSub)
	if count(r.Log, "identity create", "-n mi-demo-larkmere-mutual", "-g rg-identities", "-l westeurope") != 1 {
		t.Errorf("identity create missing: %v", r.Log)
	}
	// One grant on Foundry and one on voice: the site has both.
	if n := count(r.Log, "role assignment create", "--role Cognitive Services OpenAI User", "--assignee-object-id pid-1"); n != 2 {
		t.Errorf("want 2 grants, got %d: %v", n, r.Log)
	}
	if count(r.Log, "--scope /subscriptions/", "accounts/foundry") < 1 || count(r.Log, "role assignment create", "accounts/voice") != 1 {
		t.Errorf("grant scopes: %v", r.Log)
	}
	if n := count(r.Log, "helm", "upgrade --install workspace-larkmere-mutual", "-n omnia", "--wait --timeout 5m"); n != 1 {
		t.Errorf("helm call: %v", r.Log)
	}
	if count(r.Log, "kubectl", "label ns larkmere-mutual", "istio.io/dataplane-mode=ambient", "istio.io/use-waypoint=waypoint", "--overwrite") != 1 {
		t.Errorf("namespace not enrolled: %v", r.Log)
	}
	applied := false
	for i, l := range r.Log {
		if strings.HasSuffix(l, "apply -f -") {
			applied = true
			s := r.Stdin[i]
			if !strings.Contains(s, "namespace: larkmere-mutual") || strings.Contains(s, "__WORKSPACE_NS__") || !strings.Contains(s, "kind: Gateway") {
				t.Errorf("waypoint manifest: %q", s)
			}
		}
	}
	if !applied {
		t.Errorf("waypoint not applied: %v", r.Log)
	}
	// Order: grants, then helm, then label (enrol before agents install).
	idx := func(sub string) int {
		for i, l := range r.Log {
			if strings.Contains(l, sub) {
				return i
			}
		}
		return -1
	}
	if !(idx("identity create") < idx("role assignment create") && idx("role assignment create") < idx("upgrade --install") && idx("upgrade --install") < idx("label ns")) {
		t.Errorf("order: %v", r.Log)
	}
}

func TestTenantPassesSiteValuesToChart(t *testing.T) {
	// helm is faked and the values file is a temp file removed on exit, so
	// --values prints what would be passed (the render diff reads this too).
	r := runScriptFull(t, `export SITE="$PWD/testdata/site-full.yaml"; bash ./tenant.sh --values larkmere-mutual cid-1 tenant-1`, nil, nil)
	if r.Err != nil {
		t.Fatal(r.Err, r.Out)
	}
	for _, want := range []string{
		"name: larkmere-mutual",
		"host: pg.example.com",
		"adminLogin: pgadmin",
		"name: pg-admin",
		"endpoint: https://foundry.example.com",
		"region: swedencentral",
		"endpoint: https://voice.example.com",
		"storageAccount: stdemo",
		"enabled: true",
		"clientId: cid-1",
		"tenantId: tenant-1",
		"model: gpt-4o",
	} {
		if !strings.Contains(r.Out, want) {
			t.Errorf("values lack %q:\n%s", want, r.Out)
		}
	}
}

func TestTenantIdempotent(t *testing.T) {
	log, out, err := runScript(t, tenantScript, tenantEnv("site-full.yaml", nil), tenantExistingFixtures)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	for _, l := range log {
		if strings.Contains(l, "create") {
			t.Errorf("re-run created something: %s", l)
		}
	}
	if count(log, "role assignment list") != 2 {
		t.Errorf("grants should be checked, foundry and voice: %v", log)
	}
}

func TestTenantNoVoiceNoAmbient(t *testing.T) {
	log, out, err := runScript(t, tenantScript, tenantEnv("site-tenant-min.yaml", map[string]string{
		"FAKE_FAIL": "kubectl * get daemonset ztunnel*",
	}), tenantExistingFixtures)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "accounts/voice") != 0 {
		t.Errorf("voice touched: %v", log)
	}
	for _, l := range log {
		if strings.Contains(l, " label ") || strings.Contains(l, "apply") {
			t.Errorf("ambient step ran: %s", l)
		}
	}
	if count(log, "upgrade --install workspace-larkmere-mutual") != 1 {
		t.Errorf("helm missing: %v", log)
	}
}

func TestTenantDryRun(t *testing.T) {
	log, out, err := runScript(t, tenantScript, tenantEnv("site-full.yaml", map[string]string{
		"DRY_RUN":   "1",
		"FAKE_FAIL": "az * identity show*",
	}), tenantNewFixtures)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	assertNoWrites(t, log)
	for _, want := range []string{"DRY_RUN: az", "identity create", "DRY_RUN: helm", "DRY_RUN: kubectl"} {
		if !strings.Contains(out, want) {
			t.Errorf("out lacks %q:\n%s", want, out)
		}
	}
}

func TestTenantRemove(t *testing.T) {
	log, out, err := runScript(t, `export SITE="$PWD/testdata/site-full.yaml"; bash ./tenant.sh --remove larkmere-mutual`, nil, nil)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if len(log) != 1 || !strings.Contains(log[0], "uninstall workspace-larkmere-mutual -n omnia") {
		t.Errorf("log: %v", log)
	}
}

func TestTenantUsage(t *testing.T) {
	_, out, err := runScript(t, `export SITE="$PWD/testdata/site-full.yaml"; bash ./tenant.sh`, nil, nil)
	if exitCode(err) != 1 || !strings.Contains(out, "usage") {
		t.Errorf("want usage error, got %v %q", err, out)
	}
}

func TestTenantMissingFieldNamed(t *testing.T) {
	_, out, err := runScript(t, `export SITE="$PWD/testdata/site-min.yaml"; bash ./tenant.sh x`, nil, nil)
	if exitCode(err) != 1 || !strings.Contains(out, "site.yaml: omnia.namespace is required") {
		t.Errorf("got %v %q", err, out)
	}
}
