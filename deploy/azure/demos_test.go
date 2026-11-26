package azure

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var demoTenants = []string{"larkmere-mutual", "wrenmoor-energy", "ellens-assistant", "halvern-partners", "halvern-research"}

// siteValues runs site-values.sh for a tenant against a site file and returns
// its stdout and the combined output with the exit error.
func siteValues(t *testing.T, siteFile, tenant string) (string, error) {
	t.Helper()
	abs, err := filepath.Abs(siteFile)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "./site-values.sh", tenant)
	cmd.Env = append(os.Environ(), "SITE="+abs)
	var out bytes.Buffer
	cmd.Stdout = &out
	var errb bytes.Buffer
	cmd.Stderr = &errb
	runErr := cmd.Run()
	if runErr != nil {
		return errb.String(), runErr
	}
	return out.String(), nil
}

func TestSiteValuesLarkmere(t *testing.T) {
	out, err := siteValues(t, "testdata/site-full.yaml", "larkmere-mutual")
	if err != nil {
		t.Fatal(err, out)
	}
	for _, want := range []string{
		"00000000-0000-0000-0000-000000000001",
		"role: owner",
		"domain: larkmere.demo.example.com",
		"https://assistant.demo.example.com/agent.json",
		"issuer: https://auth.larkmere.demo.example.com",
		"issuer: https://idp.example.com/",
		"clientID: client-larkmere",
		"https://larkmere.demo.example.com",
		"audience: https://larkmere.demo.example.com/api",
		"vaultURL: https://kv-demo.vault.azure.net/",
		"gatewayName: omnia",
		"gatewayNamespace: omnia",
		"gatewaySectionName: https",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("overlay lacks %q:\n%s", want, out)
		}
	}
}

func TestSiteValuesEllensAndHalvern(t *testing.T) {
	out, err := siteValues(t, "testdata/site-full.yaml", "ellens-assistant")
	if err != nil {
		t.Fatal(err, out)
	}
	for _, want := range []string{"host: assistant.demo.example.com", "larkmere.demo.example.com", "wrenmoor.demo.example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("ellens overlay lacks %q:\n%s", want, out)
		}
	}
	out, err = siteValues(t, "testdata/site-full.yaml", "halvern-partners")
	if err != nil {
		t.Fatal(err, out)
	}
	for _, want := range []string{"name: omnia", "namespace: omnia", "dashboard.demo.example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("halvern overlay lacks %q:\n%s", want, out)
		}
	}
	out, err = siteValues(t, "testdata/site-full.yaml", "halvern-research")
	if err != nil {
		t.Fatal(err, out)
	}
	if strings.Contains(out, "poppy") || strings.Contains(out, "catalogue") {
		t.Errorf("research gets only the owners:\n%s", out)
	}
}

func TestSiteValuesMissingFieldNamesIt(t *testing.T) {
	for _, c := range []struct{ del, tenant, field string }{
		{"del(.ownerGroups)", "halvern-research", "ownerGroups"},
		{"del(.dns.zone)", "larkmere-mutual", "dns.zone"},
		{"del(.keyVault.name)", "larkmere-mutual", "keyVault.name"},
		{"del(.poppy.upstream.issuer)", "larkmere-mutual", "poppy.upstream.issuer"},
		{"del(.poppy.companies.larkmere-mutual.clientID)", "larkmere-mutual", "poppy.companies.larkmere-mutual.clientID"},
		{"del(.dashboardHost)", "halvern-partners", "dashboardHost"},
		{"del(.gateway.name)", "halvern-partners", "gateway.name"},
	} {
		_, out, err := runScript(t, `d="$(mktemp -d)"; yq e '`+c.del+`' "$PWD/testdata/site-full.yaml" > "$d/site.yaml"
export SITE="$d/site.yaml"; bash ./site-values.sh `+c.tenant, nil, nil)
		if exitCode(err) != 1 || !strings.Contains(out, "site.yaml: "+c.field+" is required") {
			t.Errorf("%s: want exit 1 naming %s, got %v %q", c.del, c.field, err, out)
		}
	}
}

// The values this repository must not carry. Built from pieces so this file
// does not match its own search.
func forbiddenValues() string {
	return strings.Join([]string{
		"0ad6" + "a315",
		"kv-omnia-demo-p-" + "gotzfbhj",
		"azure\\." + "altairalabs\\.ai",
		"dev-kvf5kf" + "vufbv6y3p6",
		"4uxuGZw33S2pR6p940" + "BENb6TCkot1lUx",
		"aEXiTb7YZgIJfxayMw" + "dP11kU52ye8ltE",
		"acromnia" + "demo",
	}, "|")
}

func TestSiteValuesHasNoCommittedTenant(t *testing.T) {
	cmd := exec.Command("git", "-C", "../..", "grep", "-n", "-E", forbiddenValues(), "--",
		"profiles", "charts", "Makefile", "deploy")
	out, err := cmd.CombinedOutput()
	if exitCode(err) == 1 {
		return // no match
	}
	t.Errorf("committed tenant values (git grep exit %d):\n%s", exitCode(err), out)
}

// renderTenant renders a tenant with the identity flags make demos passes.
func renderTenant(t *testing.T, root, tenant string, overlay string) string {
	t.Helper()
	args := []string{"template", tenant, filepath.Join(root, "charts/omnia-demo"),
		"-f", filepath.Join(root, "profiles/azure/common.yaml"),
		"-f", filepath.Join(root, "profiles/azure/"+tenant+".yaml")}
	if overlay != "" {
		args = append(args, "-f", overlay)
	}
	args = append(args, "--set", "namespace="+tenant,
		"--set-string", `workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/client-id=dummy-client`,
		"--set-string", `workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/tenant-id=dummy-tenant`)
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// TestRenderedTenantsUnchanged: the profiles plus site-values.sh render what the
// profiles at BASE_REF rendered alone. It needs deploy/azure/testdata/site-ours.yaml
// (gitignored, the deployment's real values) and BASE_REF (a commit from
// before the tenant values left the profiles); it skips without them.
func TestRenderedTenantsUnchanged(t *testing.T) {
	base := os.Getenv("BASE_REF")
	if base == "" {
		t.Skip("BASE_REF not set")
	}
	if _, err := os.Stat("testdata/site-ours.yaml"); err != nil {
		t.Skip("testdata/site-ours.yaml absent")
	}
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	baseDir := t.TempDir()
	// The chart and the profiles as they were at BASE_REF.
	arch := exec.Command("bash", "-c", `git -C ../.. archive "$BASE_REF" charts/omnia-demo profiles/azure | tar -x -C "$DIR"`)
	arch.Env = append(os.Environ(), "BASE_REF="+base, "DIR="+baseDir)
	if out, err := arch.CombinedOutput(); err != nil {
		t.Fatalf("git archive %s: %v\n%s", base, err, out)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	// The catalogue page is chart content, and it was rewritten after BASE_REF
	// to stop naming one deployment (its cluster, its start command). That is
	// an intended render change, so compare both renders on the current page
	// and let every other byte of the chart be held to BASE_REF.
	page, err := os.ReadFile(filepath.Join(root, "charts/omnia-demo/files/catalogue/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "charts/omnia-demo/files/catalogue/index.html"), page, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tenant := range demoTenants {
		t.Run(tenant, func(t *testing.T) {
			ov, err := siteValues(t, "testdata/site-ours.yaml", tenant)
			if err != nil {
				t.Fatal(err, ov)
			}
			f := filepath.Join(t.TempDir(), "overlay.yaml")
			if err := os.WriteFile(f, []byte(ov), 0o644); err != nil {
				t.Fatal(err)
			}
			want := renderTenant(t, baseDir, tenant, "")
			got := renderTenant(t, root, tenant, f)
			// Compare parsed documents: several template comments were
			// reworded to stop naming one deployment, and comments do not
			// survive parsing.
			diff := firstYAMLDiff(t, want, got)
			if diff == "" {
				return
			}
			if got != want {
				gf, wf := filepath.Join(t.TempDir(), "got"), filepath.Join(t.TempDir(), "want")
				_ = os.WriteFile(gf, []byte(got), 0o644)
				_ = os.WriteFile(wf, []byte(want), 0o644)
				d, _ := exec.Command("diff", "-u", wf, gf).CombinedOutput()
				t.Errorf("render differs from %s: %s\n%s", base, diff, d)
			}
		})
	}
}

// firstYAMLDiff decodes two renders as multi-document YAML and returns a
// description of the first document that differs, or "". Parsing drops YAML
// comments and keeps block-scalar content, so a "#" line inside an embedded
// script or fixture still counts.
func firstYAMLDiff(t *testing.T, want, got string) string {
	t.Helper()
	decode := func(s string) []map[string]any {
		var docs []map[string]any
		dec := yaml.NewDecoder(strings.NewReader(s))
		for {
			var d map[string]any
			err := dec.Decode(&d)
			if err == io.EOF {
				return docs
			}
			if err != nil {
				t.Fatalf("render is not YAML: %v", err)
			}
			if d != nil {
				docs = append(docs, d)
			}
		}
	}
	w, g := decode(want), decode(got)
	id := func(d map[string]any) string {
		md, _ := d["metadata"].(map[string]any)
		return fmt.Sprintf("%v/%v", d["kind"], md["name"])
	}
	for i := 0; i < len(w) && i < len(g); i++ {
		if !reflect.DeepEqual(w[i], g[i]) {
			return "document " + id(w[i]) + " differs"
		}
	}
	if len(w) != len(g) {
		return fmt.Sprintf("%d documents, want %d", len(g), len(w))
	}
	return ""
}

const builderScript = `export SITE="$PWD/testdata/${SITEFILE}"; bash ./builder.sh larkmere-mutual azure-gpt
`

func builderFixtures() map[string]string {
	return identityFixtures("", "", "cid-1")
}

func TestBuilderUsesSiteChart(t *testing.T) {
	log, out, err := runScript(t, builderScript, siteEnv("site-full.yaml"), builderFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	helmAt := -1
	for i, l := range log {
		if strings.HasPrefix(l, "helm ") {
			if helmAt != -1 {
				t.Errorf("more than one helm call: %v", log)
			}
			helmAt = i
		}
	}
	if helmAt != 0 {
		t.Fatalf("helm call should be first: %v", log)
	}
	want := "helm --kube-context kind-demo upgrade --install arena-builder oci://registry.example.com/charts/omnia-arena-builder --version 0.0.0 "
	if !strings.HasPrefix(log[0], want) {
		t.Errorf("helm call = %q, want prefix %q", log[0], want)
	}
	for _, s := range []string{"-n larkmere-mutual", "--set provider.name=azure-gpt",
		"--set sandbox.image.repository=registry.example.com/sandbox",
		"--set sandbox.image.tag=dev", "--set sandbox.image.digest=sha256:abc"} {
		if !strings.Contains(log[0], s) {
			t.Errorf("helm call lacks %q: %s", s, log[0])
		}
	}
	// identity.sh follows: it creates the builder's facade credential.
	if count(log[1:], "federated-credential create", "--subject system:serviceaccount:larkmere-mutual:arena-worker") != 1 {
		t.Errorf("identity did not run after the install: %v", log)
	}
}

func TestBuilderWithoutSandboxImage(t *testing.T) {
	log, out, err := runScript(t, `d="$(mktemp -d)"; yq e 'del(.builder.sandboxImage)' "$PWD/testdata/site-full.yaml" > "$d/site.yaml"
export SITE="$d/site.yaml"; bash ./builder.sh larkmere-mutual azure-gpt`, nil, builderFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	if strings.Contains(log[0], "sandbox") {
		t.Errorf("no sandboxImage, no sandbox flags: %s", log[0])
	}
}

func TestBuilderNeedsChart(t *testing.T) {
	log, out, err := runScript(t, `export SITE="$PWD/testdata/site-min.yaml"; bash ./builder.sh larkmere-mutual azure-gpt`, nil, nil)
	if exitCode(err) != 1 || !strings.Contains(out, "site.yaml: builder.chart is required") {
		t.Errorf("want exit 1 naming builder.chart, got %v %q", err, out)
	}
	if len(log) != 0 {
		t.Errorf("nothing runs before the field is checked: %v", log)
	}
}

func demosFixtures() map[string]string {
	fx := map[string]string{}
	for k, v := range tenantExistingFixtures {
		fx[k] = v
	}
	fx["*workloadIdentityClientId*"] = "cid-1"
	fx["*workloadIdentityTenantId*"] = "tenant-1"
	fx["*get_agentruntime*"] = ""
	fx["*get_sa*"] = "cid-1"
	return fx
}

const demosScript = `export SITE="$PWD/testdata/${SITEFILE}"; export CHART=../../charts/omnia-demo PROFILE_DIR=../../profiles/azure
bash ./demos.sh ${TENANT}
`

func TestDemosOrder(t *testing.T) {
	log, out, err := runScript(t, demosScript, tenantEnv("site-full.yaml", nil), demosFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	idx := func(parts ...string) int {
		for i, l := range log {
			if count([]string{l}, parts...) == 1 {
				return i
			}
		}
		return -1
	}
	tenantAt := idx("helm", "upgrade --install workspace-larkmere-mutual")
	cmAt := idx("kubectl", "get configmap omnia-workspace", "workloadIdentityClientId")
	demoAt := idx("helm", "upgrade --install larkmere-mutual ", "-n larkmere-mutual")
	idAt := idx("federated-credential create")
	if tenantAt < 0 || cmAt < 0 || demoAt < 0 || idAt < 0 || !(tenantAt < cmAt && cmAt < demoAt && demoAt < idAt) {
		t.Fatalf("order tenant=%d configmap=%d demo=%d identity=%d: %v", tenantAt, cmAt, demoAt, idAt, log)
	}
	for _, s := range []string{"profiles/azure/common.yaml", "profiles/azure/larkmere-mutual.yaml",
		"--set namespace=larkmere-mutual",
		"--set-string workspace.runtimeIdentity.serviceAccountAnnotations.azure\\.workload\\.identity/client-id=cid-1",
		"--set-string workspace.runtimeIdentity.serviceAccountAnnotations.azure\\.workload\\.identity/tenant-id=tenant-1",
		"--wait --timeout 10m"} {
		if !strings.Contains(log[demoAt], s) {
			t.Errorf("demo install lacks %q: %s", s, log[demoAt])
		}
	}
	if n := strings.Count(log[demoAt], " -f "); n != 3 {
		t.Errorf("want 3 -f (common, tenant, site overlay), got %d: %s", n, log[demoAt])
	}
}

func TestDemosUsesImagesEnv(t *testing.T) {
	log, out, err := runScript(t, `printf 'IMAGE_REGISTRY := r.example.com\nIMAGE_TAG := dev-1\n' > "$PWD/../../.images.test.env"; trap 'rm -f "$PWD/../../.images.test.env"' EXIT
export IMAGE_ENV="$PWD/../../.images.test.env"
`+demosScript, tenantEnv("site-full.yaml", nil), demosFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	found := false
	for _, l := range log {
		if strings.Contains(l, "upgrade --install larkmere-mutual ") {
			found = strings.Contains(l, "--set-string images.registry=r.example.com --set-string images.tag=dev-1")
		}
	}
	if !found {
		t.Errorf("images from .images.env not passed: %v", log)
	}
}

func TestImagesReadsSite(t *testing.T) {
	log, out, err := runScript(t, `export IMAGE_ENV="$(mktemp -d)/images.env"; export SITE="$PWD/testdata/site-full.yaml"
bash ./images.sh a,ctx-a,a/Dockerfile b,.,b/Dockerfile
cat "$IMAGE_ENV"`, nil, nil)
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "az acr build", "--subscription "+testSub, "--registry demoreg", "--image a:dev-") != 1 ||
		count(log, "az acr build", "--registry demoreg", "--image b:dev-") != 1 {
		t.Errorf("builds: %v", log)
	}
	if !strings.Contains(out, "IMAGE_REGISTRY := demoreg.azurecr.io") || !strings.Contains(out, "IMAGE_TAG := dev-") {
		t.Errorf("env file: %q", out)
	}
}

func TestImagesNeedsRegistry(t *testing.T) {
	log, out, err := runScript(t, `export SITE="$PWD/testdata/site-min.yaml"; bash ./images.sh a,.,a/Dockerfile`, nil, nil)
	if exitCode(err) != 1 || !strings.Contains(out, "site.yaml: images.registry is required") {
		t.Errorf("want exit 1 naming images.registry, got %v %q", err, out)
	}
	if len(log) != 0 {
		t.Errorf("no build without a registry: %v", log)
	}
}

// With personalAgent on, the chart names the Companies by their hosts in
// knownCompanies and refuses to render without them.
func TestAssistantNeedsKnownCompanies(t *testing.T) {
	root, _ := filepath.Abs("../..")
	out, err := exec.Command("helm", "template", "x", filepath.Join(root, "charts/omnia-demo"),
		"--set", "namespace=x", "--set", "personalAgent.enabled=true", "--set", "personalAgent.host=a.example.com",
		"--set", "personalAgent.knownCompanies={other.example.com}").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "personalAgent.knownCompanies has no larkmere. host") {
		t.Errorf("want a render failure naming knownCompanies, got %v\n%s", err, out)
	}
}
