package azure

import (
	"strings"
	"testing"
)

const identityScript = `export SITE="$PWD/testdata/${SITEFILE}"; bash ./identity.sh ${TENANT}
`

func identityEnv(extra map[string]string) map[string]string {
	return tenantEnv("site-full.yaml", extra)
}

const identityPrefix = "system:serviceaccount:larkmere-mutual:"

// identityFixtures: the identity and cluster answers. have is the tsv of
// existing credential subjects, agents the AgentRuntime facades the cluster
// lists, annotation the client id memory-api's SA currently carries.
func identityFixtures(have, agents, annotation string) map[string]string {
	return map[string]string{
		"*identity_show*": "cid-1",
		"*account_show*":  "tenant-1",
		"*aks_show*":      "https://oidc.example.com/",
		"*federated-credential_list*subject_-o_tsv*": have,
		"*federated-credential_list*gone-facade*":   "cred-gone",
		"*get_agentruntime*":                        agents,
		"*get_sa*":                                  annotation,
	}
}

func subjects(names ...string) string {
	var s []string
	for _, n := range names {
		s = append(s, identityPrefix+n)
	}
	return strings.Join(s, "\n")
}

func TestIdentityFederatesFixedAndAgents(t *testing.T) {
	log, out, err := runScript(t, identityScript, identityEnv(nil),
		identityFixtures("", "a-facade b-facade ", "cid-1"))
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if n := count(log, "federated-credential create"); n != 7 {
		t.Errorf("want 7 creates, got %d: %v", n, log)
	}
	for _, sa := range []string{"arena-worker", "eval-worker", "memory-larkmere-mutual-default",
		"privacy-larkmere-mutual", "omnia-identity-foundry", "a-facade", "b-facade"} {
		if count(log, "federated-credential create", "-g rg-identities", "--identity-name mi-demo-larkmere-mutual",
			"--name larkmere-mutual-"+sa+" ", "--issuer https://oidc.example.com/",
			"--subject "+identityPrefix+sa+" ", "--audience api://AzureADTokenExchange") != 1 {
			t.Errorf("no create for %s: %v", sa, log)
		}
	}
	if count(log, "aks show", "-g rg-aks", "-n aks-demo", "oidcIssuerProfile.issuerUrl") != 1 {
		t.Errorf("issuer lookup: %v", log)
	}
	if count(log, "delete") != 0 {
		t.Errorf("nothing to prune or delete: %v", log)
	}
}

func TestIdentityPrunesOnlyGoneFacades(t *testing.T) {
	have := subjects("a-facade", "gone-facade", "eval-worker")
	log, out, err := runScript(t, identityScript, identityEnv(nil),
		identityFixtures(have, "a-facade ", "cid-1"))
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if n := count(log, "federated-credential delete"); n != 1 {
		t.Fatalf("want exactly one delete, got %d: %v", n, log)
	}
	if count(log, "federated-credential delete", "--name cred-gone", "--identity-name mi-demo-larkmere-mutual") != 1 {
		t.Errorf("wrong delete: %v", log)
	}
	// a-facade and eval-worker exist; the other fixed SAs are still created.
	if count(log, "federated-credential create", "a-facade") != 0 || count(log, "federated-credential create", "--name larkmere-mutual-eval-worker ") != 0 {
		t.Errorf("recreated an existing credential: %v", log)
	}
}

func TestIdentityPrunesBeforeCreating(t *testing.T) {
	log, out, err := runScript(t, identityScript, identityEnv(nil),
		identityFixtures(subjects("gone-facade"), "a-facade ", "cid-1"))
	if err != nil {
		t.Fatal(err, out)
	}
	del, firstCreate := -1, -1
	for i, l := range log {
		if del < 0 && strings.Contains(l, "federated-credential delete") {
			del = i
		}
		if firstCreate < 0 && strings.Contains(l, "federated-credential create") {
			firstCreate = i
		}
	}
	if del < 0 || firstCreate < 0 || del > firstCreate {
		t.Errorf("delete (%d) must precede the first create (%d): %v", del, firstCreate, log)
	}
}

func TestIdentityPruneIgnoresForeignAndNonFacadeSubjects(t *testing.T) {
	have := "system:serviceaccount:other-ns:x-facade\n" + subjects("foo-bar")
	log, out, err := runScript(t, identityScript, identityEnv(nil),
		identityFixtures(have, "", "cid-1"))
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "federated-credential delete") != 0 {
		t.Errorf("deleted a subject it does not own: %v", log)
	}
}

func TestIdentityEmptyAgentListPrunesFacades(t *testing.T) {
	// A successful, empty list is real: every *-facade credential goes.
	log, out, err := runScript(t, identityScript, identityEnv(nil),
		identityFixtures(subjects("x-facade", "arena-worker"), "", "cid-1"))
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "federated-credential delete") != 1 {
		t.Errorf("want one delete: %v", log)
	}
}

func TestIdentityAbortsWhenAgentListFails(t *testing.T) {
	log, out, err := runScript(t, identityScript, identityEnv(map[string]string{
		"FAKE_FAIL": "kubectl * get agentruntime*",
	}), identityFixtures(subjects("a-facade", "gone-facade"), "a-facade ", "cid-1"))
	if err == nil {
		t.Fatalf("want non-zero exit, got success: %s", out)
	}
	assertContexts(t, log, testCtx, testSub)
	for _, l := range log {
		if strings.Contains(l, "delete") || strings.Contains(l, "create") {
			t.Errorf("wrote after a failed agent list: %s", l)
		}
	}
}

func TestIdentityIdempotent(t *testing.T) {
	have := subjects("arena-worker", "eval-worker", "memory-larkmere-mutual-default",
		"privacy-larkmere-mutual", "omnia-identity-foundry", "a-facade", "b-facade")
	log, out, err := runScript(t, identityScript, identityEnv(nil),
		identityFixtures(have, "a-facade b-facade ", "cid-1"))
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	assertNoWrites(t, log)
}

func TestIdentityAnnotatesMemorySAWhenClientDiffers(t *testing.T) {
	log, out, err := runScript(t, identityScript, identityEnv(nil),
		identityFixtures("", "", "old-client"))
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "annotate sa memory-larkmere-mutual-default", "-n larkmere-mutual", "--overwrite",
		"azure.workload.identity/client-id=cid-1", "azure.workload.identity/tenant-id=tenant-1") != 1 {
		t.Errorf("annotate: %v", log)
	}
	if count(log, "delete pod", "-l app.kubernetes.io/component=memory-api", "-n larkmere-mutual") != 1 {
		t.Errorf("pod delete: %v", log)
	}
	if count(log, "rollout") != 0 {
		t.Errorf("rollout restart used: %v", log)
	}
}

func TestIdentityLeavesMemoryWhenSAAbsent(t *testing.T) {
	log, out, err := runScript(t, identityScript, identityEnv(map[string]string{
		"FAKE_FAIL": "kubectl * get sa*",
	}), identityFixtures("", "", ""))
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "annotate") != 0 || count(log, "kubectl", "delete") != 0 {
		t.Errorf("touched memory-api without its SA: %v", log)
	}
}

func TestIdentityDryRun(t *testing.T) {
	log, out, err := runScript(t, identityScript, identityEnv(map[string]string{"DRY_RUN": "1"}),
		identityFixtures(subjects("gone-facade"), "a-facade ", "old-client"))
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	assertNoWrites(t, log)
	for _, want := range []string{"DRY_RUN: az", "federated-credential create", "federated-credential delete", "DRY_RUN: kubectl"} {
		if !strings.Contains(out, want) {
			t.Errorf("out lacks %q:\n%s", want, out)
		}
	}
}

func TestIdentityUsage(t *testing.T) {
	_, out, err := runScript(t, `export SITE="$PWD/testdata/site-full.yaml"; bash ./identity.sh`, nil, nil)
	if exitCode(err) != 1 || !strings.Contains(out, "usage") {
		t.Errorf("want usage error, got %v %q", err, out)
	}
}

func TestIdentityMissingFieldStopsBeforeAnyCall(t *testing.T) {
	log, out, err := runScript(t, `d="$(mktemp -d)"; yq e 'del(.aks.name)' "$PWD/testdata/site-full.yaml" > "$d/site.yaml"
export SITE="$d/site.yaml"; bash ./identity.sh larkmere-mutual`, nil, identityFixtures("", "", "cid-1"))
	if exitCode(err) != 1 || !strings.Contains(out, "site.yaml: aks.name is required") {
		t.Errorf("want exit 1 naming the field, got %v %q", err, out)
	}
	if len(log) != 0 {
		t.Errorf("calls made before the check: %v", log)
	}
}
