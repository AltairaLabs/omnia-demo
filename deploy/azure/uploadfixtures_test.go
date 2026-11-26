package azure

import (
	"crypto/md5"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The ingest Job's upload-fixtures script stands in for a company's document
// store. memory-api re-embeds a document whose modified_at moved, so the
// script must leave an unchanged document's stored object alone. These tests
// run the real script with a fake `aws` on PATH that logs each call and
// answers head-object / list-objects-v2 from a table of "key etag" lines.

const uploadScript = "../../charts/omnia-demo/files/upload-fixtures.sh"

const fakeAWS = `#!/bin/bash
echo "$*" >> "$AWS_LOG"
case " $* " in
  *" s3api head-object "*)
    key=""
    while [ $# -gt 0 ]; do [ "$1" = "--key" ] && key="$2"; shift; done
    etag=$(awk -v k="$key" '$1==k {print $2}' "$AWS_REMOTE")
    [ -n "$etag" ] || { echo "An error occurred (404) when calling the HeadObject operation" >&2; exit 254; }
    echo "\"$etag\""
    ;;
  *" s3api list-objects-v2 "*)
    if [ -s "$AWS_REMOTE" ]; then awk '{printf "%s%s", (n++ ? "\t" : ""), $1} END {print ""}' "$AWS_REMOTE"; else echo None; fi
    ;;
esac
exit 0
`

func md5hex(s string) string { return fmt.Sprintf("%x", md5.Sum([]byte(s))) }

// runUpload runs the script over fixtures (name -> content) against a fake
// bucket holding remote (key -> etag). It returns the logged aws calls.
func runUpload(t *testing.T, fixtures map[string]string, remote map[string]string) []string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	fx := filepath.Join(dir, "fixtures")
	for _, d := range []string{bin, fx, filepath.Join(fx, "..data")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(bin, "aws"), fakeAWS)
	for n, c := range fixtures {
		write(filepath.Join(fx, n), c)
	}
	// A mounted ConfigMap also holds its files under ..data/ and ..<date>/.
	write(filepath.Join(fx, "..data", "inner.md"), "inner")
	write(filepath.Join(fx, "..2026_11_26.md"), "stamped")
	var tbl strings.Builder
	for k, e := range remote {
		fmt.Fprintf(&tbl, "%s %s\n", k, e)
	}
	rf, lf := filepath.Join(dir, "remote"), filepath.Join(dir, "log")
	write(rf, tbl.String())
	write(lf, "")

	cmd := exec.Command("sh", uploadScript)
	cmd.Env = append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"),
		"AWS_LOG="+lf, "AWS_REMOTE="+rf,
		"BUCKET=b", "PREFIX=fixtures/", "FIXTURES="+fx, "ENDPOINT=http://s3:9000")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("script: %v\n%s", err, out)
	}
	b, err := os.ReadFile(lf)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// writes returns the logged calls that put or remove a stored object (the
// bulk '..*' clean-up rm is reported separately by the tests that care).
func writes(log []string) (puts, rms []string) {
	for _, l := range log {
		switch {
		case strings.Contains(l, " s3 cp "):
			puts = append(puts, l)
		case strings.Contains(l, " s3 rm ") && !strings.Contains(l, "--recursive"):
			rms = append(rms, l)
		}
	}
	return
}

func TestUploadUnchangedIsNotWritten(t *testing.T) {
	log := runUpload(t,
		map[string]string{"a.md": "alpha"},
		map[string]string{"fixtures/a.md": md5hex("alpha")})
	puts, rms := writes(log)
	if len(puts) != 0 || len(rms) != 0 {
		t.Errorf("unchanged fixture must not be written: %v %v\n%v", puts, rms, log)
	}
	if strings.Contains(strings.Join(log, "\n"), "sync") {
		t.Errorf("sync re-uploads on mtime; must not be used:\n%v", log)
	}
}

func TestUploadChangedAndNew(t *testing.T) {
	log := runUpload(t,
		map[string]string{"a.md": "alpha v2", "b.md": "beta", "same.md": "s"},
		map[string]string{"fixtures/a.md": md5hex("alpha"), "fixtures/same.md": md5hex("s")})
	puts, rms := writes(log)
	if len(puts) != 2 || len(rms) != 0 {
		t.Fatalf("want 2 uploads (changed a, new b), got puts=%v rms=%v", puts, rms)
	}
	joined := strings.Join(puts, "\n")
	if !strings.Contains(joined, "s3://b/fixtures/a.md") || !strings.Contains(joined, "s3://b/fixtures/b.md") {
		t.Errorf("wrong uploads:\n%s", joined)
	}
}

func TestUploadRemovedFixtureIsDeleted(t *testing.T) {
	log := runUpload(t,
		map[string]string{"a.md": "alpha"},
		map[string]string{"fixtures/a.md": md5hex("alpha"), "fixtures/gone.md": "x"})
	puts, rms := writes(log)
	if len(puts) != 0 || len(rms) != 1 || !strings.Contains(rms[0], "s3://b/fixtures/gone.md") {
		t.Errorf("want only gone.md removed, got puts=%v rms=%v", puts, rms)
	}
}

func TestUploadNeverSendsDotEntries(t *testing.T) {
	log := runUpload(t, map[string]string{"a.md": "alpha"}, nil)
	puts, _ := writes(log)
	for _, p := range puts {
		if strings.Contains(p, "/..") || strings.Contains(p, "inner") || strings.Contains(p, "stamped") {
			t.Errorf("a '..' entry was uploaded: %s", p)
		}
	}
	if len(puts) != 1 {
		t.Errorf("want exactly a.md uploaded, got %v", puts)
	}
	// The clean-up of earlier '..*' copies still runs.
	if !strings.Contains(strings.Join(log, "\n"), "--recursive --exclude * --include ..*") {
		t.Errorf("'..*' clean-up missing:\n%v", log)
	}
}
