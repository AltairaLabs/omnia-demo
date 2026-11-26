package azure

import (
	"strings"
	"testing"
)

// siteWith runs script after writing site-full.yaml with a yq edit applied to
// $SITE (an expression such as `del(.gateway.sectionName)`).
func siteWithScript(edit, script string) string {
	return `d="$(mktemp -d)"; yq e '` + edit + `' "$PWD/testdata/site-full.yaml" > "$d/site.yaml"
export SITE="$d/site.yaml"; ` + script
}

// --- make images -----------------------------------------------------------

// A dry run records nothing: .images.env names the tag that make demos deploys,
// and a tag that was never built would be deployed next.
func TestImagesDryRunDoesNotWriteTheEnvFile(t *testing.T) {
	log, out, err := runScript(t, `export IMAGE_ENV="$(mktemp -d)/images.env"; export SITE="$PWD/testdata/site-full.yaml"
printf 'IMAGE_TAG := old\n' > "$IMAGE_ENV"
bash ./images.sh a,ctx-a,a/Dockerfile
echo "file: $(cat "$IMAGE_ENV")"`, map[string]string{"DRY_RUN": "1"}, nil)
	if err != nil {
		t.Fatal(err, out)
	}
	if len(log) != 0 {
		t.Errorf("a dry run builds nothing: %v", log)
	}
	if !strings.Contains(out, "file: IMAGE_TAG := old") {
		t.Errorf("the env file was overwritten: %q", out)
	}
	if !strings.Contains(out, "DRY_RUN: write ") || !strings.Contains(out, "IMAGE_TAG := dev-") {
		t.Errorf("say what would be written: %q", out)
	}
}

// --- External Secrets: the databases Job's admin Secret --------------------

func TestTenantPassesDBAdminSecretInExternalSecretMode(t *testing.T) {
	edit := `.postgres.adminPassword = {"externalSecret": {"clusterSecretStore": "store-x", "key": "k", "dbAdminSecret": "my-db-admin"}}`
	r := runScriptFull(t, siteWithScript(edit, `bash ./tenant.sh --values larkmere-mutual cid-1 tenant-1`), nil, nil)
	if r.Err != nil {
		t.Fatal(r.Err, r.Out)
	}
	if !strings.Contains(r.Out, "dbBootstrap:\n  adminSecret: my-db-admin") {
		t.Errorf("dbBootstrap.adminSecret missing:\n%s", r.Out)
	}
}

// Unset, the chart's own default (omnia-db-admin) stays: nothing is written.
func TestTenantLeavesDBAdminSecretToTheChartByDefault(t *testing.T) {
	edit := `.postgres.adminPassword = {"externalSecret": {"clusterSecretStore": "store-x", "key": "k"}}`
	r := runScriptFull(t, siteWithScript(edit, `bash ./tenant.sh --values larkmere-mutual cid-1 tenant-1`), nil, nil)
	if r.Err != nil {
		t.Fatal(r.Err, r.Out)
	}
	if strings.Contains(r.Out, "dbBootstrap") {
		t.Errorf("dbBootstrap written without a site value:\n%s", r.Out)
	}
}

// --- site-values: guardrail, client Secret names, gateway section ----------

func TestSiteValuesGuardrailFromSite(t *testing.T) {
	edit := `.guardrail = {"clusterSecretStore": "store-x", "key": "gw-key"}`
	r := runScriptFull(t, siteWithScript(edit, `bash ./site-values.sh larkmere-mutual`), nil, nil)
	if r.Err != nil {
		t.Fatal(r.Err, r.Out)
	}
	for _, want := range []string{"guardrail:", "enabled: true", "clusterSecretStore: store-x", "keyVaultSecret: gw-key"} {
		if !strings.Contains(r.Out, want) {
			t.Errorf("overlay lacks %q:\n%s", want, r.Out)
		}
	}
	// Only the member assistant's tenant has the guardrail.
	r = runScriptFull(t, siteWithScript(edit, `bash ./site-values.sh wrenmoor-energy`), nil, nil)
	if r.Err != nil || strings.Contains(r.Out, "guardrail") {
		t.Errorf("wrenmoor must not get the guardrail: %v\n%s", r.Err, r.Out)
	}
}

func TestSiteValuesNoGuardrailBlockNoGuardrail(t *testing.T) {
	r := runScriptFull(t, siteWithScript(`del(.guardrail)`, `bash ./site-values.sh larkmere-mutual`), nil, nil)
	if r.Err != nil {
		t.Fatal(r.Err, r.Out)
	}
	if strings.Contains(r.Out, "guardrail") {
		t.Errorf("no guardrail block, no guardrail:\n%s", r.Out)
	}
}

func TestSiteValuesGuardrailNeedsItsFields(t *testing.T) {
	_, out, err := runScript(t, siteWithScript(`.guardrail = {"clusterSecretStore": "store-x"}`, `bash ./site-values.sh larkmere-mutual`), nil, nil)
	if exitCode(err) != 1 || !strings.Contains(out, "site.yaml: guardrail.key is required") {
		t.Errorf("want exit 1 naming guardrail.key, got %v %q", err, out)
	}
}

func TestSiteValuesClientSecretNameFromSite(t *testing.T) {
	out, err := siteValues(t, "testdata/site-full.yaml", "larkmere-mutual")
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "clientSecretRef: larkmere-upstream") {
		t.Errorf("clientSecretName not wired:\n%s", out)
	}
	out, err = siteValues(t, "testdata/site-full.yaml", "wrenmoor-energy")
	if err != nil || !strings.Contains(out, "clientSecretRef: wrenmoor-upstream") {
		t.Errorf("wrenmoor: %v\n%s", err, out)
	}
	// Unset, the profile's own name stands: nothing is emitted.
	r := runScriptFull(t, siteWithScript(`del(.poppy.companies.larkmere-mutual.clientSecretName)`, `bash ./site-values.sh larkmere-mutual`), nil, nil)
	if r.Err != nil || strings.Contains(r.Out, "clientSecretRef") {
		t.Errorf("no site value, nothing emitted: %v\n%s", r.Err, r.Out)
	}
}

func TestSiteValuesNeedsGatewaySectionName(t *testing.T) {
	for _, tenant := range []string{"larkmere-mutual", "wrenmoor-energy", "ellens-assistant", "halvern-partners"} {
		_, out, err := runScript(t, siteWithScript(`del(.gateway.sectionName)`, `bash ./site-values.sh `+tenant), nil, nil)
		if exitCode(err) != 1 || !strings.Contains(out, "site.yaml: gateway.sectionName is required") {
			t.Errorf("%s: want exit 1 naming gateway.sectionName, got %v %q", tenant, err, out)
		}
	}
}

// --- friendly messages for a missing identity ------------------------------

func TestIdentityNotFoundSaysToRunAzureTenant(t *testing.T) {
	log, out, err := runScript(t, identityScript, identityEnv(map[string]string{
		"FAKE_FAIL": "az identity show*", "FAKE_FAIL_MSG": identityNotFound,
	}), identityFixtures("", "", "cid-1"))
	if exitCode(err) != 1 || !strings.Contains(out, "not found") || !strings.Contains(out, "make azure-tenant WS=larkmere-mutual") {
		t.Fatalf("want exit 1 with the friendly message, got %v %q", err, out)
	}
	assertNoWrites(t, log)
}

func TestIdentityOtherLookupFailureIsShownAsIs(t *testing.T) {
	log, out, err := runScript(t, identityScript, identityEnv(map[string]string{
		"FAKE_FAIL": "az identity show*", "FAKE_FAIL_MSG": "(AuthorizationFailed) no rights",
	}), identityFixtures("", "", "cid-1"))
	if exitCode(err) != 1 || !strings.Contains(out, "AuthorizationFailed") || strings.Contains(out, "make azure-tenant") {
		t.Fatalf("want exit 1 showing the error and no advice to run azure-tenant, got %v %q", err, out)
	}
	assertNoWrites(t, log)
}

func TestPoppyIdentityNotFoundSaysToRunAzureTenant(t *testing.T) {
	log, out, err := runScript(t, poppyScript, map[string]string{
		"SITEFILE": "site-full.yaml", "FAKE_FAIL": "az identity show*", "FAKE_FAIL_MSG": identityNotFound,
	}, poppyFixtures())
	if exitCode(err) != 1 || !strings.Contains(out, "make azure-tenant WS=granted-a") {
		t.Fatalf("want exit 1 with the friendly message, got %v %q", err, out)
	}
	assertNoWrites(t, log)
}

func TestPoppyIdentityOtherFailureIsShownAsIs(t *testing.T) {
	log, out, err := runScript(t, poppyScript, map[string]string{
		"SITEFILE": "site-full.yaml", "FAKE_FAIL": "az identity show*", "FAKE_FAIL_MSG": "(AuthorizationFailed) no rights",
	}, poppyFixtures())
	if exitCode(err) != 1 || !strings.Contains(out, "AuthorizationFailed") || strings.Contains(out, "make azure-tenant") {
		t.Fatalf("got %v %q", err, out)
	}
	assertNoWrites(t, log)
}

func TestMemoryIdentityNotFoundSaysToRunAzureTenant(t *testing.T) {
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), map[string]string{
		"FAKE_FAIL": "az identity show*", "FAKE_FAIL_MSG": identityNotFound,
	}), memFixtures())
	if exitCode(err) != 1 || !strings.Contains(out, "make azure-tenant WS=mem-a") {
		t.Fatalf("want exit 1 with the friendly message, got %v %q", err, out)
	}
	assertNoWrites(t, log)
}

func TestMemoryIdentityOtherFailureIsShownAsIs(t *testing.T) {
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), map[string]string{
		"FAKE_FAIL": "az identity show*", "FAKE_FAIL_MSG": "(AuthorizationFailed) no rights",
	}), memFixtures())
	if exitCode(err) != 1 || !strings.Contains(out, "AuthorizationFailed") || strings.Contains(out, "make azure-tenant") {
		t.Fatalf("got %v %q", err, out)
	}
	assertNoWrites(t, log)
}

// --- identity: memory-ingest only where memory is on -----------------------

func TestIdentityFederatesMemoryIngestOnlyForMemoryTenants(t *testing.T) {
	log, out, err := runScript(t, identityScript, identityEnv(map[string]string{"TENANT": "halvern-research"}),
		identityFixtures("", "", "cid-1"))
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "federated-credential create", "--subject system:serviceaccount:halvern-research:memory-ingest ") != 0 {
		t.Errorf("halvern-research has no memory: no memory-ingest credential wanted: %v", log)
	}
	if count(log, "federated-credential create", "--subject system:serviceaccount:halvern-research:arena-worker ") != 1 {
		t.Errorf("the other fixed SAs are still federated: %v", log)
	}
	// A tenant whose profile turns memory on keeps it.
	log, _, err = runScript(t, identityScript, identityEnv(nil), identityFixtures("", "", "cid-1"))
	if err != nil || count(log, "federated-credential create", "--subject "+identityPrefix+"memory-ingest ") != 1 {
		t.Errorf("larkmere federates memory-ingest: %v %v", err, log)
	}
}

// --- identity: credentials of a recreated cluster --------------------------

const staleQuery = "*federated-credential_list*issuer!=*"

func TestIdentityRecreatesACredentialWithAStaleIssuer(t *testing.T) {
	fx := identityFixtures(subjects("a-facade", "eval-worker"), "a-facade ", "cid-1")
	fx[staleQuery] = "cred-stale\t" + identityPrefix + "a-facade"
	log, out, err := runScript(t, identityScript, identityEnv(nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if n := count(log, "federated-credential delete"); n != 1 || count(log, "federated-credential delete", "--name cred-stale", "--identity-name mi-demo-larkmere-mutual") != 1 {
		t.Fatalf("want exactly the stale credential deleted: %v", log)
	}
	if count(log, "federated-credential create", "--subject "+identityPrefix+"a-facade ", "--issuer https://oidc.example.com/") != 1 {
		t.Errorf("the stale credential must be recreated on the current issuer: %v", log)
	}
	if count(log, "federated-credential create", "--subject "+identityPrefix+"eval-worker ") != 0 {
		t.Errorf("a current credential was recreated: %v", log)
	}
	del, create := -1, -1
	for i, l := range log {
		if strings.Contains(l, "federated-credential delete") {
			del = i
		}
		if strings.Contains(l, "federated-credential create") && strings.Contains(l, "a-facade") {
			create = i
		}
	}
	if del < 0 || create < 0 || del > create {
		t.Errorf("delete (%d) must precede the recreate (%d): %v", del, create, log)
	}
}

// Credentials of other namespaces are never this script's to remove.
func TestIdentityLeavesForeignStaleIssuerCredentialsAlone(t *testing.T) {
	fx := identityFixtures("", "", "cid-1")
	fx[staleQuery] = "cred-other\tsystem:serviceaccount:other-ns:x-facade"
	log, out, err := runScript(t, identityScript, identityEnv(nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "federated-credential delete") != 0 {
		t.Errorf("deleted a credential of another namespace: %v", log)
	}
}

func TestIdentityStaleIssuerDryRun(t *testing.T) {
	fx := identityFixtures(subjects("a-facade"), "a-facade ", "cid-1")
	fx[staleQuery] = "cred-stale\t" + identityPrefix + "a-facade"
	log, out, err := runScript(t, identityScript, identityEnv(map[string]string{"DRY_RUN": "1"}), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertNoWrites(t, log)
	if !strings.Contains(out, "DRY_RUN: az identity federated-credential delete") || !strings.Contains(out, "--subject "+identityPrefix+"a-facade") {
		t.Errorf("dry run must show the delete and the recreate:\n%s", out)
	}
}

// --- poppy: the gateway's public IP may sit in another subscription --------

func TestPoppyGatewayPublicIPSubscription(t *testing.T) {
	const pipSub = "22222222-2222-2222-2222-222222222222"
	log, out, err := runScript(t, `d="$(mktemp -d)"; yq e '.dns.gatewayPublicIP.subscription = "`+pipSub+`"' "$PWD/testdata/site-poppy-pip.yaml" > "$d/site.yaml"
export SITE="$d/site.yaml"; export PROFILES_DIR="$PWD/testdata/profiles"; bash ./poppy.sh`, nil, poppyFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub, poppyDNSSub, pipSub)
	if count(log, "network public-ip show", "--subscription "+pipSub, "-g rg-net", "-n pip-gw") != 1 {
		t.Errorf("the public IP must be read in its own subscription: %v", log)
	}
}
