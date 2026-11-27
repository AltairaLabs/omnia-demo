// Package azure tests the deploy/azure scripts against fake az, kubectl and
// helm binaries (testdata/bin). Nothing here touches a cluster or a
// subscription.
package azure

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// scriptResult is the outcome of a script run. Stdin holds what each call
// received on stdin (e.g. the manifest of `kubectl apply -f -`), keyed by the
// call's index in Log; calls given no stdin are absent.
type scriptResult struct {
	Log   []string
	Out   string
	Err   error
	Stdin map[int]string
}

// runScript runs script under bash with testdata/bin first on PATH.
//
// The fakes append "<tool> <args>" to $FAKE_LOG, so log is every call in
// order. A read is answered from fixtures: the map key is the call's args
// joined with "_" (spaces and "/" become "_"), or a glob matched against that
// key (e.g. "*identity_show*"). A call whose "<tool> <args>" matches the glob
// in env["FAKE_FAIL"] exits 3, so absent-resource paths can be tested.
// out is stdout and stderr combined. err is non-nil for a non-zero exit.
// SITE, DRY_RUN and FAKE_* are scrubbed from the inherited environment.
func runScript(t *testing.T, script string, env map[string]string, fixtures map[string]string) (log []string, out string, err error) {
	t.Helper()
	r := runScriptFull(t, script, env, fixtures)
	return r.Log, r.Out, r.Err
}

// runScriptFull is runScript that also returns the stdin each call received.
func runScriptFull(t *testing.T, script string, env map[string]string, fixtures map[string]string) scriptResult {
	t.Helper()
	dir := t.TempDir()
	fixDir := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(fixDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for k, v := range fixtures {
		if err := os.WriteFile(filepath.Join(fixDir, k+".json"), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	logFile := filepath.Join(dir, "calls.log")
	if err := os.WriteFile(logFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	scriptFile := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(scriptFile, []byte("set -euo pipefail\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin, err := filepath.Abs("testdata/bin")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptFile)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name == "SITE" || name == "DRY_RUN" || strings.HasPrefix(name, "FAKE_") {
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	cmd.Env = append(cmd.Env,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_LOG="+logFile,
		"FAKE_FIXTURES="+fixDir,
	)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	b, runErr := cmd.CombinedOutput()
	res := scriptResult{Out: string(b), Err: runErr, Stdin: map[int]string{}}
	raw, _ := os.ReadFile(logFile)
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if l != "" {
			res.Log = append(res.Log, l)
		}
	}
	for i := range res.Log {
		// The fakes write stdin to <log>.stdin.<n>, n counting from 1.
		if s, rErr := os.ReadFile(logFile + ".stdin." + strconv.Itoa(i+1)); rErr == nil {
			res.Stdin[i] = string(s)
		}
	}
	return res
}

func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 0
}

// flagValue returns the token after flag in a logged call, or "".
func flagValue(call, flag string) string {
	f := strings.Fields(call)
	for i, w := range f {
		if w == flag && i+1 < len(f) {
			return f[i+1]
		}
	}
	return ""
}

// contextProblems reports each call that does not name its target, or names
// the wrong one: kubectl needs --context kubeContext, helm --kube-context
// kubeContext, az --subscription one of subscriptions.
func contextProblems(log []string, kubeContext string, subscriptions []string) []string {
	var bad []string
	for _, l := range log {
		tool, _, _ := strings.Cut(l, " ")
		switch tool {
		case "kubectl":
			if flagValue(l, "--context") != kubeContext {
				bad = append(bad, l)
			}
		case "helm":
			if flagValue(l, "--kube-context") != kubeContext {
				bad = append(bad, l)
			}
		case "az":
			ok := false
			for _, s := range subscriptions {
				if flagValue(l, "--subscription") == s {
					ok = true
				}
			}
			if !ok {
				bad = append(bad, l)
			}
		}
	}
	return bad
}

// assertContexts fails when a call does not name its target with the expected
// value. Pass every subscription the script may legitimately use (the main
// one, and dns.subscription when it differs).
func assertContexts(t *testing.T, log []string, kubeContext string, subscriptions ...string) {
	t.Helper()
	for _, l := range contextProblems(log, kubeContext, subscriptions) {
		t.Errorf("call without the expected context/subscription: %s", l)
	}
}

var writeVerbs = map[string]bool{
	"create": true, "delete": true, "update": true, "set": true, "add": true,
	"assign": true, "apply": true, "patch": true, "annotate": true, "label": true,
	"replace": true, "install": true, "upgrade": true, "uninstall": true,
	"rollout": true, "scale": true, "put": true, "post": true,
}

// writesIn returns the logged calls that write. A call writes when:
//   - any token is a write verb (create, delete, apply, install, ...);
//   - any token has an add- or set- prefix (add-record, set-record, ...);
//   - it is `az rest` and its --method is anything but get.
func writesIn(log []string) []string {
	var w []string
	for _, l := range log {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		hit := false
		isRest := false
		for _, tok := range f[1:] {
			tok = strings.ToLower(tok)
			if writeVerbs[tok] || strings.HasPrefix(tok, "add-") || strings.HasPrefix(tok, "set-") {
				hit = true
			}
			if tok == "rest" {
				isRest = true
			}
		}
		if f[0] == "az" && isRest {
			if m := strings.ToLower(flagValue(l, "--method")); m != "" && m != "get" {
				hit = true
			}
		}
		if hit {
			w = append(w, l)
		}
	}
	return w
}

// assertNoWrites fails when any logged call is a write (DRY_RUN must not
// execute them; reads may still run).
func assertNoWrites(t *testing.T, log []string) {
	t.Helper()
	for _, l := range writesIn(log) {
		t.Errorf("write executed: %s", l)
	}
}

const libScript = `export SITE="$PWD/testdata/${SITEFILE}"; source ./lib.sh
`

func siteEnv(file string) map[string]string { return map[string]string{"SITEFILE": file} }

const (
	testSub = "00000000-0000-0000-0000-000000000000"
	testCtx = "kind-demo"
)

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

func TestSiteGetHyphenatedKey(t *testing.T) {
	_, out, err := runScript(t, libScript+`site_get poppy.companies.larkmere-mutual.clientID`, siteEnv("site-full.yaml"), nil)
	if err != nil || strings.TrimSpace(out) != "client-larkmere" {
		t.Errorf("got %q, %v", out, err)
	}
}

func TestSiteHasOptional(t *testing.T) {
	_, out, err := runScript(t, libScript+`site_has voice`, siteEnv("site-min.yaml"), nil)
	if exitCode(err) != 1 {
		t.Errorf("site_has voice on site-min: want exit 1, got %v", err)
	}
	if strings.Contains(out, "is required") {
		t.Errorf("site_has must not complain: %q", out)
	}
	_, out, err = runScript(t, libScript+`site_has voice`, siteEnv("site-full.yaml"), nil)
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
	log, _, err := runScript(t, libScript+`run azm identity create x`, siteEnv("site-min.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0] != "az identity create x --subscription "+testSub {
		t.Errorf("log: %v", log)
	}
	assertContexts(t, log, testCtx, testSub)
}

// DRY_RUN prints the az command exactly as it would run, --subscription last.
func TestDryRunPrintsAzWithSubscriptionLast(t *testing.T) {
	log, out, err := runScript(t, libScript+`DRY_RUN=1 run azm identity create x
DRY_RUN=1 run az_in other-sub group delete -n rg`, siteEnv("site-min.yaml"), nil)
	if err != nil {
		t.Fatal(err, out)
	}
	for _, want := range []string{
		"DRY_RUN: az identity create x --subscription " + testSub + "\n",
		"DRY_RUN: az group delete -n rg --subscription other-sub\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("out lacks %q: %q", want, out)
		}
	}
	if len(log) != 0 {
		t.Errorf("fake called: %v", log)
	}
}

// The real az rejects a global argument before the command group; the fake
// must too, or a wrong argument order passes every test.
func TestFakeAzRejectsLeadingGlobalArgument(t *testing.T) {
	_, out, err := runScript(t, `az --subscription x identity show`, nil, nil)
	if exitCode(err) != 2 || !strings.Contains(out, "'--subscription' is misspelled or not recognized") {
		t.Errorf("want exit 2 and the az error, got %v %q", err, out)
	}
	_, out, err = runScript(t, `az identity show -n y --subscription x`, nil, nil)
	if err != nil {
		t.Errorf("trailing --subscription must be accepted: %v %q", err, out)
	}
}

func TestToolArraysCarryContext(t *testing.T) {
	log, _, err := runScript(t, libScript+`
"${KUBECTL[@]}" get ns
"${HELM[@]}" list
azm group show -n rg`, siteEnv("site-min.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 3 {
		t.Fatalf("log: %v", log)
	}
	assertContexts(t, log, testCtx, testSub)
	assertNoWrites(t, log)
	if log[0] != "kubectl --context kind-demo get ns" || log[1] != "helm --kube-context kind-demo list" {
		t.Errorf("log: %v", log)
	}
}

func TestContextProblemsFlagsWrongValues(t *testing.T) {
	log := []string{
		"kubectl --context kind-demo get ns",
		"kubectl --context other get ns",
		"kubectl get ns",
		"helm --kube-context other list",
		"az --subscription " + testSub + " group show",
		"az --subscription dns-sub group show",
		"az --subscription nope group show",
		"az group show",
	}
	bad := contextProblems(log, testCtx, []string{testSub, "dns-sub"})
	want := []string{log[1], log[2], log[3], log[6], log[7]}
	if strings.Join(bad, "|") != strings.Join(want, "|") {
		t.Errorf("got %v want %v", bad, want)
	}
}

func TestWritesInFlagsEveryWriteShape(t *testing.T) {
	writes := []string{
		"az --subscription s rest --method put --url https://x",
		"az --subscription s rest --method POST --url https://x",
		"az --subscription s rest --method delete --url https://x",
		"az --subscription s network dns record-set a add-record -g rg",
		"az --subscription s network dns record-set a set-record -g rg",
		"az --subscription s identity create -n x",
		"kubectl --context c apply -f -",
		"helm --kube-context c upgrade --install r chart",
	}
	reads := []string{
		"az --subscription s rest --method get --url https://x",
		"az --subscription s rest --url https://x",
		"az --subscription s identity show -n x",
		"az --subscription s network dns record-set a show -g rg",
		"kubectl --context c get ns",
		"helm --kube-context c list",
	}
	got := writesIn(append(append([]string{}, writes...), reads...))
	if strings.Join(got, "|") != strings.Join(writes, "|") {
		t.Errorf("writes = %v\nwant %v", got, writes)
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

func TestFakeCapturesStdin(t *testing.T) {
	r := runScriptFull(t, `kubectl get ns
kubectl apply -f - <<'EOF'
kind: ConfigMap
EOF
helm template x`, nil, nil)
	if r.Err != nil {
		t.Fatal(r.Err, r.Out)
	}
	if len(r.Log) != 3 || r.Log[1] != "kubectl apply -f -" {
		t.Fatalf("log: %v", r.Log)
	}
	if got := r.Stdin[1]; got != "kind: ConfigMap\n" {
		t.Errorf("stdin of call 1 = %q", got)
	}
	if _, ok := r.Stdin[0]; ok {
		t.Error("call 0 had no stdin")
	}
	if _, ok := r.Stdin[2]; ok {
		t.Error("call 2 had no stdin")
	}
}

func TestRunScriptScrubsInheritedEnv(t *testing.T) {
	t.Setenv("DRY_RUN", "1")
	t.Setenv("FAKE_FAIL", "*")
	t.Setenv("SITE", "/nonexistent")
	log, out, err := runScript(t, `az group show
echo "[${DRY_RUN:-}][${SITE:-}]"`, nil, nil)
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "[][]") || len(log) != 1 {
		t.Errorf("env leaked: out=%q log=%v", out, log)
	}
}
