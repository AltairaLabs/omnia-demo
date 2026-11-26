package azure

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

const workspaceChart = "../../charts/omnia-workspace"

// baseSets is the smallest values set that renders the chart in
// externalSecret mode, with optional blocks (voice, nfs) off.
func baseSets(extra ...string) []string {
	s := []string{
		"name=larkmere-mutual",
		"postgres.host=pg.example.com",
		"postgres.adminLogin=pgadmin",
		"postgres.password.externalSecret.store=azure-kv",
		"foundry.endpoint=https://foundry.example.com",
		"foundry.region=eastus2",
		"nfs.enabled=false",
	}
	return append(s, extra...)
}

// helmTemplate renders the workspace chart with the given --set values. It
// only renders: helm never talks to a cluster here.
func helmTemplate(t *testing.T, sets ...string) (string, error) {
	t.Helper()
	args := []string{"template", "workspace-larkmere-mutual", workspaceChart, "--namespace", "omnia"}
	for _, s := range sets {
		args = append(args, "--set", s)
	}
	b, err := exec.Command("helm", args...).CombinedOutput()
	return string(b), err
}

func mustTemplate(t *testing.T, sets ...string) string {
	t.Helper()
	out, err := helmTemplate(t, sets...)
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	return out
}

var (
	kindRe = regexp.MustCompile(`(?m)^kind: (\S+)$`)
	nameRe = regexp.MustCompile(`(?m)^  name: (\S+)$`)
)

// docs maps "Kind/name" to each rendered document.
func docs(out string) map[string]string {
	m := map[string]string{}
	for _, d := range strings.Split(out, "\n---\n") {
		k := kindRe.FindStringSubmatch(d)
		n := nameRe.FindStringSubmatch(d)
		if k != nil && n != nil {
			m[k[1]+"/"+n[1]] = d
		}
	}
	return m
}

func countKind(out, kind string) int {
	n := 0
	for k := range docs(out) {
		if strings.HasPrefix(k, kind+"/") {
			n++
		}
	}
	return n
}

func TestWorkspaceChartNames(t *testing.T) {
	d := docs(mustTemplate(t, baseSets()...))
	for _, want := range []string{
		"Namespace/larkmere-mutual",
		"ConfigMap/omnia-workspace",
		"ExternalSecret/omnia-postgres",
		"ExternalSecret/omnia-postgres-memory",
		"ExternalSecret/omnia-postgres-privacy",
		"Job/workspace-larkmere-mutual-db",
	} {
		if _, ok := d[want]; !ok {
			t.Errorf("missing %s; have %v", want, keys(d))
		}
	}
	job := d["Job/workspace-larkmere-mutual-db"]
	for _, db := range []string{"omnia_sessions_larkmere_mutual", "omnia_memory_larkmere_mutual", "omnia_consent_larkmere_mutual"} {
		if !strings.Contains(job, db) {
			t.Errorf("db Job lacks %s", db)
		}
	}
	// The remote key defaults to the one the live deployment uses.
	if es := d["ExternalSecret/omnia-postgres"]; !strings.Contains(es, "key: omnia-postgres-admin-password") || !strings.Contains(es, "name: azure-kv") {
		t.Errorf("ExternalSecret store/key wrong:\n%s", es)
	}
}

func TestWorkspaceChartPasswordModes(t *testing.T) {
	ext := mustTemplate(t, baseSets()...)
	if n := countKind(ext, "ExternalSecret"); n != 3 {
		t.Errorf("externalSecret mode: want 3 ExternalSecrets, got %d", n)
	}
	if _, ok := docs(ext)["Job/workspace-larkmere-mutual-secrets"]; ok {
		t.Error("externalSecret mode must not render the secrets Job")
	}

	ref := mustTemplate(t,
		"name=larkmere-mutual", "postgres.host=pg.example.com", "postgres.adminLogin=pgadmin",
		"postgres.password.secretRef.name=omnia-db-admin", "postgres.password.secretRef.key=PGPASSWORD",
		"foundry.endpoint=https://foundry.example.com", "foundry.region=eastus2", "nfs.enabled=false")
	if n := countKind(ref, "ExternalSecret"); n != 0 {
		t.Errorf("secretRef mode: want no ExternalSecret, got %d", n)
	}
	d := docs(ref)
	job, ok := d["Job/workspace-larkmere-mutual-secrets"]
	if !ok {
		t.Fatalf("secretRef mode: secrets Job missing; have %v", keys(d))
	}
	for _, want := range []string{
		"helm.sh/hook: post-install,post-upgrade",
		"omnia-postgres-memory", "omnia-postgres-privacy",
		"omnia_sessions_larkmere_mutual", "omnia_consent_larkmere_mutual",
		"POSTGRES_CONN", "omnia-db-admin",
	} {
		if !strings.Contains(job, want) {
			t.Errorf("secrets Job lacks %q:\n%s", want, job)
		}
	}
	// The Job's rights: its Role is in the tenant namespace and may only
	// touch Secrets; the one read in the release namespace names the Secret.
	role := d["Role/workspace-larkmere-mutual-secrets"]
	if !strings.Contains(role, "namespace: larkmere-mutual") || !strings.Contains(role, "secrets") ||
		strings.Contains(role, "'*'") || strings.Contains(role, `"*"`) {
		t.Errorf("tenant Role wrong:\n%s", role)
	}
	rd := d["Role/workspace-larkmere-mutual-read-admin"]
	if !strings.Contains(rd, "namespace: omnia") || !strings.Contains(rd, "- omnia-db-admin") {
		t.Errorf("admin-read Role wrong:\n%s", rd)
	}

	both, err := helmTemplate(t, baseSets("postgres.password.secretRef.name=x", "postgres.password.secretRef.key=k")...)
	if err == nil || !strings.Contains(both, "exactly one") {
		t.Errorf("both forms should fail with 'exactly one': %v\n%s", err, both)
	}
	none, err := helmTemplate(t, "name=larkmere-mutual", "postgres.host=h", "postgres.adminLogin=a", "nfs.enabled=false")
	if err == nil || !strings.Contains(none, "exactly one") {
		t.Errorf("neither form should fail with 'exactly one': %v\n%s", err, none)
	}
}

func TestWorkspaceChartOptional(t *testing.T) {
	base := mustTemplate(t, baseSets()...)
	if strings.Contains(base, "azure-foundry-gpt-realtime") {
		t.Error("voice Providers rendered without voice.endpoint")
	}
	if !strings.Contains(base, "azure-foundry-gpt-4o") {
		t.Error("foundry Providers missing")
	}
	for _, k := range []string{"PersistentVolume", "PersistentVolumeClaim"} {
		if countKind(base, k) != 0 {
			t.Errorf("nfs.enabled=false still renders %s", k)
		}
	}
	if _, ok := docs(base)["Job/workspace-larkmere-mutual-subtree"]; ok {
		t.Error("nfs.enabled=false still renders the subtree Job")
	}

	voice := mustTemplate(t, baseSets("voice.endpoint=https://voice.example.com", "voice.region=swedencentral")...)
	if !strings.Contains(voice, "azure-foundry-gpt-realtime") {
		t.Error("voice.endpoint set but voice Providers missing")
	}

	nfs := mustTemplate(t, baseSets("nfs.enabled=true", "nfs.storageAccount=acct")...)
	d := docs(nfs)
	for _, want := range []string{"PersistentVolume/workspace-larkmere-mutual-content-nfs", "PersistentVolumeClaim/workspace-larkmere-mutual-content", "Job/workspace-larkmere-mutual-subtree"} {
		if _, ok := d[want]; !ok {
			t.Errorf("nfs enabled: missing %s", want)
		}
	}
	if !strings.Contains(d["Job/workspace-larkmere-mutual-subtree"], "chown 65532:65532") {
		t.Error("subtree Job must chown to 65532")
	}
}

func keys(m map[string]string) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}
