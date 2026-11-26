// Package azure tests the deploy/azure scripts against fake az, kubectl and
// helm binaries (testdata/bin). Nothing here touches a cluster or a
// subscription.
package azure

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runScript runs script under bash with testdata/bin first on PATH.
//
// The fakes append "<tool> <args>" to $FAKE_LOG, so log is every call in
// order. A read is answered from fixtures: the map key is the call's args
// joined with "_" (spaces and "/" become "_"), or a glob matched against that
// key (e.g. "*identity_show*"). A call whose "<tool> <args>" matches the glob
// in env["FAKE_FAIL"] exits 3, so absent-resource paths can be tested.
// out is stdout and stderr combined. err is non-nil for a non-zero exit.
func runScript(t *testing.T, script string, env map[string]string, fixtures map[string]string) (log []string, out string, err error) {
	t.Helper()
	dir := t.TempDir()
	fixDir := filepath.Join(dir, "fixtures")
	if mkErr := os.MkdirAll(fixDir, 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	for k, v := range fixtures {
		if wErr := os.WriteFile(filepath.Join(fixDir, k+".json"), []byte(v), 0o644); wErr != nil {
			t.Fatal(wErr)
		}
	}
	logFile := filepath.Join(dir, "calls.log")
	if wErr := os.WriteFile(logFile, nil, 0o644); wErr != nil {
		t.Fatal(wErr)
	}
	scriptFile := filepath.Join(dir, "script.sh")
	if wErr := os.WriteFile(scriptFile, []byte("set -euo pipefail\n"+script+"\n"), 0o755); wErr != nil {
		t.Fatal(wErr)
	}
	bin, absErr := filepath.Abs("testdata/bin")
	if absErr != nil {
		t.Fatal(absErr)
	}
	cmd := exec.Command("bash", scriptFile)
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_LOG="+logFile,
		"FAKE_FIXTURES="+fixDir,
	)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	b, runErr := cmd.CombinedOutput()
	raw, _ := os.ReadFile(logFile)
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if l != "" {
			log = append(log, l)
		}
	}
	return log, string(b), runErr
}

func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 0
}

// assertContexts fails when a call does not name its target: kubectl needs
// --context, helm --kube-context, az --subscription.
func assertContexts(t *testing.T, log []string) {
	t.Helper()
	need := map[string]string{
		"kubectl": " --context ",
		"helm":    " --kube-context ",
		"az":      " --subscription ",
	}
	for _, l := range log {
		tool, _, _ := strings.Cut(l, " ")
		flag, ok := need[tool]
		if ok && !strings.Contains(l, flag) {
			t.Errorf("call without%s: %s", flag, l)
		}
	}
}

var writeVerbs = map[string]bool{
	"create": true, "delete": true, "update": true, "set": true, "add": true,
	"assign": true, "apply": true, "patch": true, "annotate": true, "label": true,
	"replace": true, "install": true, "upgrade": true, "uninstall": true,
	"rollout": true, "scale": true,
}

// assertNoWrites fails when any logged call is a write (DRY_RUN must not
// execute them; reads may still run).
func assertNoWrites(t *testing.T, log []string) {
	t.Helper()
	for _, l := range log {
		for _, w := range strings.Fields(l)[1:] {
			if writeVerbs[w] {
				t.Errorf("write executed: %s", l)
				break
			}
		}
	}
}

const libScript = `export SITE="$PWD/testdata/${SITEFILE}"; source ./lib.sh
`

func siteEnv(file string) map[string]string { return map[string]string{"SITEFILE": file} }

func TestSiteReqMissingFieldNamesIt(t *testing.T) {
	_, out, err := runScript(t, libScript+`site_req postgres.host`, siteEnv("site-min.yaml"), nil)
	if exitCode(err) != 1 {
		t.Fatalf("want exit 1, got %v (%s)", err, out)
	}
	if !strings.Contains(out, "site.yaml: postgres.host is required") {
		t.Errorf("message missing: %q", out)
	}
}

func TestSiteReqPrintsValue(t *testing.T) {
	_, out, err := runScript(t, libScript+`site_req postgres.host`, siteEnv("site-full.yaml"), nil)
	if err != nil || strings.TrimSpace(out) != "pg.example.com" {
		t.Errorf("got %q, %v", out, err)
	}
}

func TestSiteGetAbsentIsEmpty(t *testing.T) {
	_, out, err := runScript(t, libScript+`echo "[$(site_get voice.endpoint)]"`, siteEnv("site-min.yaml"), nil)
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("got %q, %v", out, err)
	}
}

func TestSiteHasOptional(t *testing.T) {
	_, _, err := runScript(t, libScript+`site_has voice`, siteEnv("site-min.yaml"), nil)
	if exitCode(err) == 0 {
		t.Error("site_has voice should be false on site-min")
	}
	_, out, err := runScript(t, libScript+`site_has voice`, siteEnv("site-full.yaml"), nil)
	if err != nil {
		t.Errorf("site_has voice should be true on site-full: %v %s", err, out)
	}
}

func TestDryRunPrintsWrites(t *testing.T) {
	log, out, err := runScript(t, libScript+`DRY_RUN=1 run az identity create x`, siteEnv("site-min.yaml"), nil)
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "DRY_RUN: az identity create x") {
		t.Errorf("out: %q", out)
	}
	if len(log) != 0 {
		t.Errorf("fake called: %v", log)
	}
}

func TestRunExecutesWithoutDryRun(t *testing.T) {
	log, _, err := runScript(t, libScript+`run "${AZ[@]}" identity create x`, siteEnv("site-min.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0] != "az --subscription 00000000-0000-0000-0000-000000000000 identity create x" {
		t.Errorf("log: %v", log)
	}
	assertContexts(t, log)
}

func TestToolArraysCarryContext(t *testing.T) {
	log, _, err := runScript(t, libScript+`
"${KUBECTL[@]}" get ns
"${HELM[@]}" list
"${AZ[@]}" group show -n rg`, siteEnv("site-min.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 3 {
		t.Fatalf("log: %v", log)
	}
	assertContexts(t, log)
	assertNoWrites(t, log)
	if log[0] != "kubectl --context kind-demo get ns" || log[1] != "helm --kube-context kind-demo list" {
		t.Errorf("log: %v", log)
	}
}

func TestFakeFixtureByKeyAndGlob(t *testing.T) {
	_, out, err := runScript(t, `az identity show -n x
az group show -n rg`, nil, map[string]string{
		"identity_show_-n_x": `{"clientId":"abc"}`,
		"*group_show*":       `{"name":"rg"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"clientId":"abc"`) || !strings.Contains(out, `"name":"rg"`) {
		t.Errorf("out: %q", out)
	}
}

func TestFakeFailExits3(t *testing.T) {
	log, _, err := runScript(t, `az identity show -n x`, map[string]string{"FAKE_FAIL": "az identity show*"}, nil)
	if exitCode(err) != 3 {
		t.Errorf("want exit 3, got %v", err)
	}
	if len(log) != 1 {
		t.Errorf("failed call still logged: %v", log)
	}
}
