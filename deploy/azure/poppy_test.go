package azure

import (
	"strings"
	"testing"
)

const (
	poppyDNSSub = "11111111-1111-1111-1111-111111111111"
	vaultID     = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-kv/providers/Microsoft.KeyVault/vaults/kv-demo"
	keyScope    = vaultID + "/keys/omnia-connections"
)

// poppyScript runs poppy.sh against the fixture profiles in testdata/profiles.
const poppyScript = `export SITE="$PWD/testdata/${SITEFILE}"; export PROFILES_DIR="$PWD/testdata/profiles"; bash ./poppy.sh
`

// poppyRealScript runs it against the repo's own profiles/azure.
const poppyRealScript = `export SITE="$PWD/testdata/${SITEFILE}"; bash ./poppy.sh
`

func poppyFixtures() map[string]string {
	return map[string]string{
		"*keyvault_show*":        vaultID,
		"*network_public-ip*":    "198.51.100.7",
		"*identity_show*":        "pid-1",
		"*role_assignment_list*": "",
	}
}

func TestPoppyDNS(t *testing.T) {
	log, out, err := runScript(t, poppyScript, siteEnv("site-poppy-pip.yaml"), poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub, poppyDNSSub)
	if n := count(log, "record-set a add-record"); n != 5 {
		t.Fatalf("want 5 add-record calls, got %d: %v", n, log)
	}
	for _, h := range []string{"larkmere", "wrenmoor", "auth.larkmere", "auth.wrenmoor", "assistant"} {
		for _, verb := range []string{"delete", "add-record"} {
			if count(log, "--subscription "+poppyDNSSub, "record-set a "+verb, "-g rg-dns", "-z demo.example.com", "-n "+h+" ") != 1 {
				t.Errorf("no %s for %s: %v", verb, h, log)
			}
		}
	}
	if n := count(log, "add-record", "-a 198.51.100.7", "--ttl 300"); n != 5 {
		t.Errorf("want 5 records at the gateway IP with ttl 300, got %d: %v", n, log)
	}
	if count(log, "network public-ip show", "--subscription "+testSub, "-g rg-net", "-n pip-gw", "--query ipAddress") != 1 {
		t.Errorf("gateway IP lookup: %v", log)
	}
	// Each record is replaced: its delete comes before its add.
	for _, h := range []string{"larkmere", "assistant"} {
		d, a := -1, -1
		for i, l := range log {
			if strings.Contains(l, "record-set a delete") && strings.Contains(l, "-n "+h+" ") {
				d = i
			}
			if strings.Contains(l, "record-set a add-record") && strings.Contains(l, "-n "+h+" ") {
				a = i
			}
		}
		if d < 0 || a < 0 || d > a {
			t.Errorf("%s: delete at %d, add at %d", h, d, a)
		}
	}
}

func TestPoppyDNSAddsWhenRecordSetIsNotFound(t *testing.T) {
	log, out, err := runScript(t, poppyScript, map[string]string{
		"SITEFILE": "site-poppy-pip.yaml", "FAKE_FAIL": "az * record-set a show*",
		"FAKE_FAIL_MSG": "(ResourceNotFound) The Resource 'Microsoft.Network/dnszones/x/A/larkmere' was not found.",
	}, poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub, poppyDNSSub)
	if count(log, "record-set a delete") != 0 {
		t.Errorf("nothing to delete: %v", log)
	}
	if n := count(log, "record-set a add-record"); n != 5 {
		t.Errorf("want 5 adds, got %d", n)
	}
}

func TestPoppyDNSStopsOnOtherShowFailure(t *testing.T) {
	log, out, err := runScript(t, poppyScript, map[string]string{
		"SITEFILE": "site-poppy-pip.yaml", "FAKE_FAIL": "az * record-set a show*",
		"FAKE_FAIL_MSG": "(AuthorizationFailed) The client does not have authorization.",
	}, poppyFixtures())
	if err == nil {
		t.Fatalf("want a failure, got success: %s", out)
	}
	if !strings.Contains(out, "AuthorizationFailed") {
		t.Errorf("the error must be shown: %q", out)
	}
	if count(log, "add-record") != 0 || count(log, "record-set a delete") != 0 {
		t.Errorf("must not write after a failed lookup: %v", log)
	}
}

func TestPoppyDNSDeletesExistingBeforeAdd(t *testing.T) {
	log, out, err := runScript(t, poppyScript, siteEnv("site-poppy-pip.yaml"), poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	for _, h := range []string{"larkmere", "wrenmoor", "auth.larkmere", "auth.wrenmoor", "assistant"} {
		if count(log, "--subscription "+poppyDNSSub, "record-set a show", "-g rg-dns", "-z demo.example.com", "-n "+h+" ") != 1 {
			t.Errorf("no show for %s: %v", h, log)
		}
	}
	if n := count(log, "record-set a delete"); n != 5 {
		t.Errorf("want 5 deletes, got %d", n)
	}
}

func TestPoppyDeleteFailureStops(t *testing.T) {
	log, out, err := runScript(t, poppyScript, map[string]string{
		"SITEFILE": "site-poppy-pip.yaml", "FAKE_FAIL": "az * record-set a delete*",
		"FAKE_FAIL_MSG": "throttled",
	}, poppyFixtures())
	if err == nil {
		t.Fatalf("a failed delete must stop the script: %s", out)
	}
	if count(log, "add-record") != 0 {
		t.Errorf("added after a failed delete: %v", log)
	}
}

func TestPoppyGatewayIPFromSite(t *testing.T) {
	log, out, err := runScript(t, poppyScript, siteEnv("site-full.yaml"), poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "public-ip") != 0 {
		t.Errorf("dns.gatewayIP is set, no lookup wanted: %v", log)
	}
	if n := count(log, "add-record", "-a 203.0.113.10"); n != 5 {
		t.Errorf("want 5 records at dns.gatewayIP, got %d", n)
	}
}

func TestPoppyNeedsAGatewayIP(t *testing.T) {
	log, out, err := runScript(t, poppyScript, siteEnv("site-poppy-noip.yaml"), poppyFixtures())
	if exitCode(err) != 1 || !strings.Contains(out, "site.yaml: dns.gatewayIP is required") {
		t.Fatalf("want exit 1 naming dns.gatewayIP, got %v (%s)", err, out)
	}
	assertNoWrites(t, log)
}

func TestPoppyKeyOnlyWhenMissing(t *testing.T) {
	// Missing: az resource show fails.
	log, out, err := runScript(t, poppyScript, map[string]string{
		"SITEFILE": "site-full.yaml", "FAKE_FAIL": "az * resource show*",
	}, poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if n := count(log, "rest --method put"); n != 1 {
		t.Fatalf("want one put when the key is missing, got %d: %v", n, log)
	}
	if count(log, "rest --method put", "--url https://management.azure.com"+keyScope+"?api-version=2023-07-01",
		`"keySize":3072`, `"keyOps":["wrapKey","unwrapKey"]`, `"kty":"RSA"`) != 1 {
		t.Errorf("put request: %v", log)
	}
	if count(log, "keyvault show", "-g rg-kv", "--name kv-demo") != 1 || count(log, "resource show", "--ids "+keyScope) != 1 {
		t.Errorf("vault/key lookups: %v", log)
	}

	// Present: az resource show succeeds.
	log, out, err = runScript(t, poppyScript, siteEnv("site-full.yaml"), poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	if n := count(log, "rest --method put"); n != 0 {
		t.Errorf("key exists, want no put, got %d: %v", n, log)
	}
}

func TestPoppyGrantsFromProfiles(t *testing.T) {
	log, out, err := runScript(t, poppyScript, siteEnv("site-full.yaml"), poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if n := count(log, "role assignment create"); n != 2 {
		t.Fatalf("want 2 grants, got %d: %v", n, log)
	}
	for _, tn := range []string{"granted-a", "granted-b"} {
		if count(log, "identity show", "-g rg-identities", "-n mi-demo-"+tn+" ", "--query principalId") != 1 {
			t.Errorf("no principal lookup for %s: %v", tn, log)
		}
	}
	if count(log, "role assignment create", "--assignee-object-id pid-1", "--assignee-principal-type ServicePrincipal",
		"--role Key Vault Crypto User", "--scope "+keyScope) != 2 {
		t.Errorf("grant shape: %v", log)
	}
	for _, tn := range []string{"other-kms", "auth-off", "poppy-off", "no-poppy", "common"} {
		if count(log, "mi-demo-"+tn) != 0 {
			t.Errorf("%s must not be granted: %v", tn, log)
		}
	}
	if !strings.Contains(out, "vault-url https://kv-demo.vault.azure.net/") {
		t.Errorf("vault-url missing: %q", out)
	}
}

func TestPoppyGrantsRealProfiles(t *testing.T) {
	log, out, err := runScript(t, poppyRealScript, siteEnv("site-full.yaml"), poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	var who []string
	for _, l := range log {
		if strings.Contains(l, "identity show") {
			who = append(who, flagValue(l, "-n"))
		}
	}
	if strings.Join(who, ",") != "mi-demo-larkmere-mutual,mi-demo-wrenmoor-energy" {
		t.Errorf("grants for %v, want exactly larkmere-mutual and wrenmoor-energy", who)
	}
	if n := count(log, "role assignment create"); n != 2 {
		t.Errorf("want 2 grants, got %d", n)
	}
}

func TestPoppyGrantIsNoOpWhenHeld(t *testing.T) {
	fx := poppyFixtures()
	fx["*role_assignment_list*"] = "/assignment/1"
	log, out, err := runScript(t, poppyScript, siteEnv("site-full.yaml"), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "role assignment list", "--scope "+keyScope, "--role Key Vault Crypto User") != 2 {
		t.Errorf("want 2 existence checks: %v", log)
	}
	if count(log, "role assignment create") != 0 {
		t.Errorf("already granted: %v", log)
	}
}

func TestPoppyDryRunWritesNothing(t *testing.T) {
	log, out, err := runScript(t, poppyScript, map[string]string{
		"SITEFILE": "site-poppy-pip.yaml", "DRY_RUN": "1", "FAKE_FAIL": "az * resource show*",
	}, poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub, poppyDNSSub)
	assertNoWrites(t, log)
	for _, want := range []string{"DRY_RUN: az", "record-set a add-record", "rest --method put", "role assignment create"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output lacks %q: %s", want, out)
		}
	}
}
