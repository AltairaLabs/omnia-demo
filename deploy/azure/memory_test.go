package azure

import (
	"crypto/md5" //nolint:gosec // Blob's contentMd5 is an MD5; this is not a security use.
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	memAcctID    = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-docs/providers/Microsoft.Storage/storageAccounts/stdocs"
	memContainer = "mem-a-documents"
	memScope     = memAcctID + "/blobServices/default/containers/" + memContainer
)

func md5b64(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec
	return base64.StdEncoding.EncodeToString(sum[:])
}

// memDocs writes memory-documents-shaped fixtures (tenant mem-a: a.md and b.md)
// and returns the directory.
func memDocs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "mem-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	for n, c := range map[string]string{"a.md": "alpha\n", "b.md": "bravo\n"} {
		if err := os.WriteFile(filepath.Join(dir, "mem-a", n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// memFixtures: every blob already holds the current content, nothing to grant
// is held yet unless a test says so.
func memFixtures() map[string]string {
	return map[string]string{
		"*storage_account_show*":        memAcctID,
		"*storage_blob_show*_-n_a.md_*": md5b64("alpha\n"),
		"*storage_blob_show*_-n_b.md_*": md5b64("bravo\n"),
		"*storage_blob_list*":           "a.md\nb.md",
		"*identity_show*":               "pid-1",
		"*role_assignment_list*":        "/assignment/1",
		"*get_jobs*":                    "memory-ingest-1 memory-ingest",
	}
}

func memEnv(docs string, extra map[string]string) map[string]string {
	env := map[string]string{
		"SITEFILE":     "site-full.yaml",
		"DOCS_DIR":     docs,
		"PROFILES_DIR": "",
	}
	env["PROFILES_DIR"], _ = filepath.Abs("testdata/memprofiles")
	for k, v := range extra {
		env[k] = v
	}
	return env
}

const memScript = `export SITE="$PWD/testdata/${SITEFILE}"; bash ./memory.sh
`

func TestMemoryRerunWithNothingChangedWritesNothing(t *testing.T) {
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), memFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	assertNoWrites(t, log)
	if n := count(log, "storage blob show"); n != 2 {
		t.Errorf("want a hash check per file (2), got %d: %v", n, log)
	}
	// An idempotent re-run also starts no ingest run.
	if count(log, "create job") != 0 {
		t.Errorf("nothing changed, nothing to start: %v", log)
	}
}

func TestMemoryUploadsOnlyAChangedFile(t *testing.T) {
	fx := memFixtures()
	fx["*storage_blob_show*_-n_a.md_*"] = md5b64("an older alpha\n")
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if n := count(log, "storage blob upload"); n != 1 {
		t.Fatalf("want one upload, got %d: %v", n, log)
	}
	if count(log, "storage blob upload", "--auth-mode login", "--account-name stdocs", "-c "+memContainer,
		"-n a.md ", "--overwrite", "--content-md5 "+md5b64("alpha\n"), "--content-type text/markdown") != 1 {
		t.Errorf("upload shape: %v", log)
	}
	if count(log, "storage blob delete") != 0 || count(log, "role assignment create") != 0 || count(log, "container create") != 0 {
		t.Errorf("only the upload should write: %v", writesIn(log))
	}
}

func TestMemoryUploadsANewFile(t *testing.T) {
	log, out, err := runScript(t, memScript, map[string]string{
		"SITEFILE": "site-full.yaml", "DOCS_DIR": memDocs(t),
		"PROFILES_DIR":  mustAbs(t, "testdata/memprofiles"),
		"FAKE_FAIL":     "az storage blob show*-n b.md *",
		"FAKE_FAIL_MSG": "(BlobNotFound) The specified blob does not exist.",
	}, memFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "storage blob upload", "-n b.md ") != 1 || count(log, "storage blob upload") != 1 {
		t.Errorf("want one upload, of b.md: %v", log)
	}
}

func TestMemoryDeletesARemovedDocument(t *testing.T) {
	fx := memFixtures()
	fx["*storage_blob_list*"] = "a.md\nb.md\ngone.md"
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "storage blob delete") != 1 ||
		count(log, "storage blob delete", "--auth-mode login", "--account-name stdocs", "-c "+memContainer, "-n gone.md") != 1 {
		t.Errorf("want one delete of gone.md: %v", log)
	}
	if count(log, "storage blob upload") != 0 {
		t.Errorf("unchanged files must not upload: %v", log)
	}
}

func TestMemoryCreatesAMissingContainerPrivate(t *testing.T) {
	log, out, err := runScript(t, memScript, map[string]string{
		"SITEFILE": "site-full.yaml", "DOCS_DIR": memDocs(t),
		"PROFILES_DIR":  mustAbs(t, "testdata/memprofiles"),
		"FAKE_FAIL":     "az storage container show*",
		"FAKE_FAIL_MSG": "(ContainerNotFound) The specified container does not exist.",
	}, memFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "storage container create", "--auth-mode login", "--account-name stdocs", "-n "+memContainer+" ", "--public-access off") != 1 {
		t.Errorf("container create: %v", log)
	}
	if count(log, "--public-access") != 1 || count(log, "--public-access blob") != 0 || count(log, "--public-access container") != 0 {
		t.Errorf("a container must never be public: %v", log)
	}
}

func TestMemoryStopsWhenTheContainerLookupIsRefused(t *testing.T) {
	log, out, err := runScript(t, memScript, map[string]string{
		"SITEFILE": "site-full.yaml", "DOCS_DIR": memDocs(t),
		"PROFILES_DIR":  mustAbs(t, "testdata/memprofiles"),
		"FAKE_FAIL":     "az storage container show*",
		"FAKE_FAIL_MSG": "(AuthorizationPermissionMismatch) This request is not authorized.",
	}, memFixtures())
	if exitCode(err) != 1 || !strings.Contains(out, "AuthorizationPermissionMismatch") {
		t.Fatalf("want exit 1 showing the error, got %v (%s)", err, out)
	}
	assertNoWrites(t, log)
}

func TestMemoryStopsWhenABlobLookupIsRefused(t *testing.T) {
	log, out, err := runScript(t, memScript, map[string]string{
		"SITEFILE": "site-full.yaml", "DOCS_DIR": memDocs(t),
		"PROFILES_DIR":  mustAbs(t, "testdata/memprofiles"),
		"FAKE_FAIL":     "az storage blob show*",
		"FAKE_FAIL_MSG": "(AuthorizationFailed) no rights",
	}, memFixtures())
	if exitCode(err) != 1 {
		t.Fatalf("want exit 1, got %v (%s)", err, out)
	}
	assertNoWrites(t, log)
}

func TestMemoryGrantScopeIsTheContainer(t *testing.T) {
	fx := memFixtures()
	fx["*role_assignment_list*"] = ""
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "role assignment create") != 1 {
		t.Fatalf("want one grant: %v", log)
	}
	if count(log, "role assignment create", "--assignee-object-id pid-1", "--assignee-principal-type ServicePrincipal",
		"--role Storage Blob Data Reader", "--scope "+memScope+" ") != 1 {
		t.Errorf("grant shape: %v", log)
	}
	for _, l := range log {
		if strings.Contains(l, "role assignment") && flagValue(l, "--scope") == memAcctID {
			t.Errorf("a grant or check scoped to the whole account: %s", l)
		}
	}
	if count(log, "identity show", "-g rg-identities", "-n mi-demo-mem-a ", "--query principalId") != 1 {
		t.Errorf("principal lookup: %v", log)
	}
}

func TestMemoryHeldGrantIsNotRepeated(t *testing.T) {
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), memFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "role assignment list", "--scope "+memScope, "--role Storage Blob Data Reader") != 1 {
		t.Errorf("want one existence check on the container: %v", log)
	}
	if count(log, "role assignment create") != 0 {
		t.Errorf("already held: %v", log)
	}
}

func TestMemoryWithoutABlockIsANoOp(t *testing.T) {
	log, out, err := runScript(t, memScript, map[string]string{"SITEFILE": "site-min.yaml"}, nil)
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "no memory block in site.yaml: nothing to do") {
		t.Errorf("out: %q", out)
	}
	if len(log) != 0 {
		t.Errorf("want zero calls, got %v", log)
	}
}

func TestMemoryDryRunWritesNothing(t *testing.T) {
	fx := memFixtures()
	fx["*storage_blob_show*_-n_a.md_*"] = "stale"
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), map[string]string{
		"DRY_RUN": "1", "FAKE_FAIL": "az storage container show*",
		"FAKE_FAIL_MSG": "(ContainerNotFound) does not exist",
	}), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	assertNoWrites(t, log)
	for _, want := range []string{"DRY_RUN: az storage container create", "DRY_RUN: az storage blob upload", "DRY_RUN: kubectl"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output lacks %q: %s", want, out)
		}
	}
}

func TestMemoryStartsARunWhenTheCronJobExists(t *testing.T) {
	fx := memFixtures()
	fx["*storage_blob_show*_-n_a.md_*"] = "stale"
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "kubectl", "-n mem-a ", "get cronjob memory-ingest") != 1 {
		t.Errorf("cronjob check: %v", log)
	}
	if count(log, "kubectl", "-n mem-a ", "create job --from=cronjob/memory-ingest memory-ingest-") != 1 {
		t.Errorf("job create: %v", log)
	}
}

func TestMemoryNoJobWhenTheCronJobIsAbsent(t *testing.T) {
	fx := memFixtures()
	fx["*storage_blob_show*_-n_a.md_*"] = "stale"
	env := memEnv(memDocs(t), map[string]string{"FAKE_FAIL": "kubectl*get cronjob*"})
	log, out, err := runScript(t, memScript, env, fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "create job") != 0 {
		t.Errorf("no CronJob, no job: %v", log)
	}
	if !strings.Contains(out, "make demos first") {
		t.Errorf("should say the CronJob is not installed: %q", out)
	}
}

func TestMemoryTenantsComeFromProfiles(t *testing.T) {
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), memFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	var who []string
	for _, l := range log {
		if strings.Contains(l, "identity show") {
			who = append(who, flagValue(l, "-n"))
		}
	}
	if strings.Join(who, ",") != "mi-demo-mem-a" {
		t.Errorf("tenants %v, want only mem-a (mem-off, no-memory and common are not tenants)", who)
	}
}

func TestMemoryRealProfiles(t *testing.T) {
	docs, _ := filepath.Abs("../../memory-documents")
	log, out, err := runScript(t, memScript, map[string]string{
		"SITEFILE": "site-full.yaml", "DOCS_DIR": docs,
		"FAKE_FAIL": "az storage blob show*", "FAKE_FAIL_MSG": "(BlobNotFound) does not exist",
	}, memFixtures())
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
		t.Errorf("tenants %v, want exactly larkmere-mutual and wrenmoor-energy", who)
	}
	for _, c := range []string{"larkmere-mutual-documents", "wrenmoor-energy-documents"} {
		if count(log, "storage blob upload", "-c "+c+" ") == 0 {
			t.Errorf("no upload into %s: %v", c, log)
		}
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestMemoryStartsTheFirstRunWhenTheCronJobHasNeverRun(t *testing.T) {
	fx := memFixtures()
	fx["*get_jobs*"] = ""
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "create job --from=cronjob/memory-ingest memory-ingest-") != 1 {
		t.Errorf("never ran, nothing changed: want one job: %v", log)
	}
	if len(writesIn(log)) != 1 {
		t.Errorf("only the job should write: %v", writesIn(log))
	}
}

func TestMemoryStartsNothingWhenAJobAlreadyRan(t *testing.T) {
	// Matched by owner name alone as well as by name prefix.
	fx := memFixtures()
	fx["*get_jobs*"] = "manual-run-9 memory-ingest"
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "create job") != 0 {
		t.Errorf("a Job exists, nothing changed: %v", log)
	}
}

func TestMemoryDoesNotStartAJobRightAfterANewGrant(t *testing.T) {
	fx := memFixtures()
	fx["*role_assignment_list*"] = ""
	fx["*get_jobs*"] = ""
	log, out, err := runScript(t, memScript, memEnv(memDocs(t), nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "role assignment create") != 1 {
		t.Fatalf("want the grant: %v", log)
	}
	if count(log, "create job") != 0 {
		t.Errorf("a Job would 403 before the grant propagates: %v", log)
	}
	if !strings.Contains(out, "few minutes to propagate") || !strings.Contains(out, "Re-run make azure-memory") {
		t.Errorf("should say to wait and re-run: %q", out)
	}
}

func TestMemoryIgnoresAzWarningsOnStderr(t *testing.T) {
	env := memEnv(memDocs(t), map[string]string{"FAKE_STDERR": "WARNING: a deprecation notice"})
	log, out, err := runScript(t, memScript, env, memFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertNoWrites(t, log)
}

func TestMemoryRefusesAnEmptyDocumentsDirectory(t *testing.T) {
	docs := t.TempDir()
	if err := os.MkdirAll(filepath.Join(docs, "mem-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	log, out, err := runScript(t, memScript, memEnv(docs, nil), memFixtures())
	if exitCode(err) != 1 || !strings.Contains(out, "no documents in") {
		t.Fatalf("want exit 1 naming the empty directory, got %v (%s)", err, out)
	}
	assertNoWrites(t, log)
	if count(log, "storage blob list") != 0 {
		t.Errorf("must stop before looking at blobs: %v", log)
	}
}

// memDocsNested is memDocs plus member-guidance/g.md, and dotfiles that are
// not documents (.hidden, .git/x.md).
func memDocsNested(t *testing.T) string {
	t.Helper()
	dir := memDocs(t)
	for p, c := range map[string]string{
		"member-guidance/g.md": "guide\n",
		".hidden":              "x\n",
		".git/x.md":            "x\n",
	} {
		full := filepath.Join(dir, "mem-a", p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func memNestedFixtures() map[string]string {
	fx := memFixtures()
	fx["*storage_blob_show*_-n_member-guidance_g.md_*"] = md5b64("guide\n")
	fx["*storage_blob_list*"] = "a.md\nb.md\nmember-guidance/g.md"
	return fx
}

func TestMemoryUploadsANestedFileUnderItsFolder(t *testing.T) {
	fx := memNestedFixtures()
	fx["*storage_blob_show*_-n_member-guidance_g.md_*"] = "stale"
	log, out, err := runScript(t, memScript, memEnv(memDocsNested(t), nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "storage blob upload") != 1 ||
		count(log, "storage blob upload", "-c "+memContainer, "-n member-guidance/g.md ", "--content-md5 "+md5b64("guide\n")) != 1 {
		t.Errorf("want one upload of member-guidance/g.md: %v", log)
	}
	if count(log, "storage blob delete") != 0 {
		t.Errorf("a listed nested blob with a file must not be deleted: %v", log)
	}
}

func TestMemoryNestedUnchangedWritesNothing(t *testing.T) {
	log, out, err := runScript(t, memScript, memEnv(memDocsNested(t), nil), memNestedFixtures())
	if err != nil {
		t.Fatal(err, out)
	}
	assertNoWrites(t, log)
	// a.md, b.md and the nested one are checked; the dotfiles are not documents.
	if n := count(log, "storage blob show"); n != 3 {
		t.Errorf("want a hash check per document (3), got %d: %v", n, log)
	}
}

func TestMemoryDeletesANestedBlobWithNoFile(t *testing.T) {
	fx := memNestedFixtures()
	fx["*storage_blob_list*"] = "a.md\nb.md\nmember-guidance/g.md\nmember-guidance/gone.md"
	log, out, err := runScript(t, memScript, memEnv(memDocsNested(t), nil), fx)
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "storage blob delete") != 1 || count(log, "storage blob delete", "-n member-guidance/gone.md") != 1 {
		t.Errorf("want one delete of member-guidance/gone.md: %v", log)
	}
}

func TestMemoryRefusesADirectoryHoldingOnlyDotfiles(t *testing.T) {
	docs := t.TempDir()
	if err := os.MkdirAll(filepath.Join(docs, "mem-a", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "mem-a", ".git", "x.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	log, out, err := runScript(t, memScript, memEnv(docs, nil), memFixtures())
	if exitCode(err) != 1 || !strings.Contains(out, "no documents in") {
		t.Fatalf("want exit 1, got %v (%s)", err, out)
	}
	assertNoWrites(t, log)
}

const publishScript = `export SITE="$PWD/testdata/${SITEFILE}"; bash ../../hack/publish-document.sh "$DOC" mem-a member-guidance/
`

func publishDoc(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "flood-claims.md")
	if err := os.WriteFile(p, []byte("flood\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPublishDocumentUploadsToTheFolderAndStartsARun(t *testing.T) {
	log, out, err := runScript(t, publishScript, map[string]string{"SITEFILE": "site-full.yaml", "DOC": publishDoc(t)}, nil)
	if err != nil {
		t.Fatal(err, out)
	}
	assertContexts(t, log, testCtx, testSub)
	if count(log, "storage blob upload", "--auth-mode login", "--account-name stdocs", "-c "+memContainer,
		"-n member-guidance/flood-claims.md ", "--overwrite", "--content-md5 "+md5b64("flood\n"), "--content-type text/markdown") != 1 {
		t.Errorf("upload shape: %v", log)
	}
	if count(log, "kubectl", "-n mem-a ", "create job --from=cronjob/memory-ingest memory-ingest-publish-") != 1 {
		t.Errorf("job create: %v", log)
	}
	// writesIn does not know "upload" as a write verb, so only the job counts.
	if len(log) != 2 || len(writesIn(log)) != 1 {
		t.Errorf("want exactly the upload and the job: %v", log)
	}
}

func TestPublishDocumentHonoursKubeContext(t *testing.T) {
	log, out, err := runScript(t, publishScript, map[string]string{"SITEFILE": "site-full.yaml", "DOC": publishDoc(t), "KUBE_CONTEXT": "other-ctx"}, nil)
	if err != nil {
		t.Fatal(err, out)
	}
	if count(log, "kubectl --context other-ctx", "create job") != 1 {
		t.Errorf("want the override context: %v", log)
	}
}

func TestPublishDocumentDryRunWritesNothing(t *testing.T) {
	log, out, err := runScript(t, publishScript, map[string]string{"SITEFILE": "site-full.yaml", "DOC": publishDoc(t), "DRY_RUN": "1"}, nil)
	if err != nil {
		t.Fatal(err, out)
	}
	assertNoWrites(t, log)
	for _, want := range []string{"DRY_RUN: az storage blob upload", "DRY_RUN: kubectl"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output lacks %q: %s", want, out)
		}
	}
}

func TestPublishDocumentNeedsAMemoryBlock(t *testing.T) {
	log, out, err := runScript(t, publishScript, map[string]string{"SITEFILE": "site-min.yaml", "DOC": publishDoc(t)}, nil)
	if exitCode(err) == 0 || !strings.Contains(out, "memory") {
		t.Fatalf("want a failure naming the memory block, got %v (%s)", err, out)
	}
	if len(log) != 0 {
		t.Errorf("want zero calls, got %v", log)
	}
}

func TestPublishDocumentNeedsAFile(t *testing.T) {
	log, out, err := runScript(t, publishScript, map[string]string{"SITEFILE": "site-full.yaml", "DOC": "/no/such/file.md"}, nil)
	if exitCode(err) == 0 || !strings.Contains(out, "no such file") {
		t.Fatalf("want a failure, got %v (%s)", err, out)
	}
	if len(log) != 0 {
		t.Errorf("want zero calls, got %v", log)
	}
}

const publishFolderScript = `export SITE="$PWD/testdata/${SITEFILE}"; bash ../../hack/publish-document.sh "$DOC" mem-a "$FOLDER"
`

func TestPublishDocumentRejectsAnUnsafeFolder(t *testing.T) {
	for _, folder := range []string{"../x", "a/../b", "a/./b", ".", "..", "/", "///", "a//b", "a b", "a/b c", "a/é", "a;b", "a\\b"} {
		t.Run(folder, func(t *testing.T) {
			log, out, err := runScript(t, publishFolderScript,
				map[string]string{"SITEFILE": "site-full.yaml", "DOC": publishDoc(t), "FOLDER": folder}, nil)
			if exitCode(err) != 1 || !strings.Contains(out, "folder") {
				t.Fatalf("want exit 1 naming the folder, got %v (%s)", err, out)
			}
			if len(log) != 0 {
				t.Errorf("want zero calls, got %v", log)
			}
		})
	}
}

func TestPublishDocumentAcceptsANestedFolder(t *testing.T) {
	for folder, blob := range map[string]string{
		"member-guidance/2026": "member-guidance/2026/flood-claims.md",
		"//member-guidance//":  "member-guidance/flood-claims.md",
		"/a.b/c_d-e/":          "a.b/c_d-e/flood-claims.md",
	} {
		log, out, err := runScript(t, publishFolderScript,
			map[string]string{"SITEFILE": "site-full.yaml", "DOC": publishDoc(t), "FOLDER": folder}, nil)
		if err != nil {
			t.Fatal(folder, err, out)
		}
		if count(log, "storage blob upload", "-n "+blob+" ") != 1 {
			t.Errorf("folder %q: want an upload as %s: %v", folder, blob, log)
		}
	}
}

const publishArgsScript = `export SITE="$PWD/testdata/${SITEFILE}"; bash ../../hack/publish-document.sh "$DOC" "$TENANT" "$FOLDER"
`

func TestPublishDocumentRejectsUnsafeArguments(t *testing.T) {
	named := func(name string) string {
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cases := map[string]struct{ doc, tenant, folder string }{
		"newline folder":         {named("ok.md"), "mem-a", "a\n../b"},
		"newline then ok folder": {named("ok.md"), "mem-a", "ok\nok"},
		"tenant with slash":      {named("ok.md"), "mem-a/x", "f"},
		"tenant uppercase":       {named("ok.md"), "Mem-A", "f"},
		"empty tenant chars":     {named("ok.md"), "mem_a", "f"},
		"basename with space":    {named("a b.md"), "mem-a", "f"},
		"basename with newline":  {named("a\nb.md"), "mem-a", "f"},
		"dot basename":           {named(".hidden.md"), "mem-a", "f"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			log, out, err := runScript(t, publishArgsScript, map[string]string{
				"SITEFILE": "site-full.yaml", "DOC": c.doc, "TENANT": c.tenant, "FOLDER": c.folder}, nil)
			if exitCode(err) != 1 {
				t.Fatalf("want exit 1, got %v (%s)", err, out)
			}
			if len(log) != 0 {
				t.Errorf("want zero calls, got %v", log)
			}
		})
	}
}
