package azure

import (
	"io"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const demoChart = "../../charts/omnia-demo"

// renderDemoChart renders the omnia-demo chart with the given --set-string
// values and returns its documents by "Kind/name". helm only renders here.
func renderDemoChart(t *testing.T, sets ...string) (map[string]map[string]any, string) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	args := []string{"template", "memtest", demoChart, "--set-string", "namespace=memtest"}
	for _, s := range sets {
		args = append(args, "--set", s)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return decodeDocs(t, string(out)), string(out)
}

func decodeDocs(t *testing.T, s string) map[string]map[string]any {
	t.Helper()
	m := map[string]map[string]any{}
	dec := yaml.NewDecoder(strings.NewReader(s))
	for {
		var d map[string]any
		err := dec.Decode(&d)
		if err == io.EOF {
			return m
		}
		if err != nil {
			t.Fatalf("render is not YAML: %v", err)
		}
		if d == nil {
			continue
		}
		md, _ := d["metadata"].(map[string]any)
		m[docKey(d["kind"], md["name"])] = d
	}
}

func docKey(kind, name any) string {
	return strings.TrimSpace(strings.Join([]string{asString(kind), asString(name)}, "/"))
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// at walks nested maps and slices by key or index.
func at(v any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, _ := v.(map[string]any)
			v = m[k]
		case int:
			s, _ := v.([]any)
			if k >= len(s) {
				return nil
			}
			v = s[k]
		}
	}
	return v
}

var memorySets = []string{
	"memory.enabled=true",
	"memory.blob.accountURL=https://stexample.blob.core.windows.net",
	"memory.blob.container=larkmere-mutual-documents",
	`workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/client-id=cid-1`,
	`workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/tenant-id=tid-1`,
}

func TestMemoryCronJobSpec(t *testing.T) {
	docs, _ := renderDemoChart(t, memorySets...)
	cj := docs["CronJob/memory-ingest"]
	if cj == nil {
		t.Fatalf("no CronJob/memory-ingest; got %v", keysOf(docs))
	}
	if got := at(cj, "spec", "schedule"); got != "0 * * * *" {
		t.Errorf("default schedule = %v", got)
	}
	if got := at(cj, "spec", "concurrencyPolicy"); got != "Forbid" {
		t.Errorf("concurrencyPolicy = %v", got)
	}
	pod := at(cj, "spec", "jobTemplate", "spec", "template")
	if got := at(pod, "metadata", "labels", "azure.workload.identity/use"); got != "true" {
		t.Errorf("workload identity pod label = %v", got)
	}
	if got := at(pod, "spec", "serviceAccountName"); got != "memory-ingest" {
		t.Errorf("serviceAccountName = %v", got)
	}
	env := map[string]string{}
	for _, e := range at(pod, "spec", "containers", 0, "env").([]any) {
		env[asString(at(e, "name"))] = asString(at(e, "value"))
	}
	for k, want := range map[string]string{
		"BLOB_ACCOUNT_URL": "https://stexample.blob.core.windows.net",
		"BLOB_CONTAINER":   "larkmere-mutual-documents",
		"MEMORY_SOURCE":    "blob",
		"WORKSPACE_ID":     "memtest",
	} {
		if env[k] != want {
			t.Errorf("env %s = %q, want %q", k, env[k], want)
		}
	}
	if env["MEMORY_API_URL"] == "" {
		t.Error("MEMORY_API_URL is empty")
	}
	if env["MEMORY_TOKEN_PATH"] == "" {
		t.Error("MEMORY_TOKEN_PATH is empty: serviceAuth is on by default")
	}
	for k := range env {
		if strings.HasPrefix(k, "S3_") || strings.HasPrefix(k, "AWS_") || k == "SOURCE_DIR" {
			t.Errorf("env %s: the S3 and directory paths are gone", k)
		}
	}
	sa := docs["ServiceAccount/memory-ingest"]
	if sa == nil {
		t.Fatalf("no ServiceAccount/memory-ingest; got %v", keysOf(docs))
	}
	if got := at(sa, "metadata", "annotations", "azure.workload.identity/client-id"); got != "cid-1" {
		t.Errorf("SA client-id = %v", got)
	}
	if got := at(sa, "metadata", "annotations", "azure.workload.identity/tenant-id"); got != "tid-1" {
		t.Errorf("SA tenant-id = %v", got)
	}
}

func TestMemoryCronJobScheduleOverride(t *testing.T) {
	docs, _ := renderDemoChart(t, append([]string{"memory.schedule=*/15 * * * *"}, memorySets...)...)
	if got := at(docs["CronJob/memory-ingest"], "spec", "schedule"); got != "*/15 * * * *" {
		t.Errorf("schedule = %v", got)
	}
}

// Review Focus 5: nothing ingests on upgrade, so no ingest resource is a Helm
// hook, and the old hook Job is gone.
func TestMemoryIngestHasNoHelmHook(t *testing.T) {
	docs, out := renderDemoChart(t, memorySets...)
	for key, d := range docs {
		if !strings.Contains(key, "memory-ingest") {
			continue
		}
		if got := at(d, "metadata", "annotations", "helm.sh/hook"); got != nil {
			t.Errorf("%s carries helm.sh/hook %v", key, got)
		}
	}
	if _, ok := docs["Job/omnia-demo-memory-ingest"]; ok {
		t.Error("the hook Job omnia-demo-memory-ingest still renders")
	}
	for _, gone := range []string{"memory-s3", "omnia-demo-memory-fixtures", "versitygw"} {
		if strings.Contains(out, gone) {
			t.Errorf("render still mentions %q", gone)
		}
	}
}

// Review Focus 4: memory is optional.
func TestMemoryOffRendersNoIngest(t *testing.T) {
	docs, _ := renderDemoChart(t, "memory.enabled=false")
	for key := range docs {
		if strings.Contains(key, "memory-ingest") {
			t.Errorf("memory off still renders %s", key)
		}
	}
}

func TestMemoryOnWithoutBlobFailsLoudly(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	out, err := exec.Command("helm", "template", "memtest", demoChart,
		"--set-string", "namespace=memtest", "--set", "memory.enabled=true").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "memory.blob.accountURL") {
		t.Errorf("want a failure naming memory.blob.accountURL, got %v\n%s", err, out)
	}
}

func keysOf(m map[string]map[string]any) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}

func TestSiteValuesMemoryBlock(t *testing.T) {
	for _, tenant := range []string{"larkmere-mutual", "wrenmoor-energy"} {
		out, err := siteValues(t, "testdata/site-full.yaml", tenant)
		if err != nil {
			t.Fatal(err, out)
		}
		for _, want := range []string{
			"accountURL: https://stdocs.blob.core.windows.net",
			"container: " + tenant + "-documents",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s overlay lacks %q:\n%s", tenant, want, out)
			}
		}
		if strings.Contains(out, "enabled: false") {
			t.Errorf("%s: memory stays on with a memory block:\n%s", tenant, out)
		}
	}
}

func TestSiteValuesWithoutMemoryBlockTurnsMemoryOff(t *testing.T) {
	for _, tenant := range []string{"larkmere-mutual", "wrenmoor-energy"} {
		_, out, err := runScript(t, `d="$(mktemp -d)"; yq e 'del(.memory)' "$PWD/testdata/site-full.yaml" > "$d/site.yaml"
export SITE="$d/site.yaml"; bash ./site-values.sh `+tenant, nil, nil)
		if err != nil {
			t.Fatal(err, out)
		}
		if !strings.Contains(out, "memory:") || !strings.Contains(out, "enabled: false") ||
			strings.Contains(out, "blob") {
			t.Errorf("%s: want memory.enabled false and no blob values:\n%s", tenant, out)
		}
	}
}

func TestSiteValuesMemoryUntouchedForTenantsWithoutMemory(t *testing.T) {
	for _, tenant := range []string{"ellens-assistant", "halvern-partners", "halvern-research"} {
		out, err := siteValues(t, "testdata/site-full.yaml", tenant)
		if err != nil {
			t.Fatal(err, out)
		}
		if strings.Contains(out, "memory") {
			t.Errorf("%s has no memory in its profile, so none in its overlay:\n%s", tenant, out)
		}
	}
}
