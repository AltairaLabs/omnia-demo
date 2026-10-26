package identity

import (
	"encoding/json"
	"os"
	"testing"
)

// testdata/pseudonym_vectors.json is a verbatim copy of Omnia's
// pkg/identity/testdata/pseudonym_vectors.json at AltairaLabs/Omnia
// 6d8aea78566562f51fc77ad8d6522a7207ab9ac5. Every implementation of the
// pseudonym and the key fingerprint must reproduce every row. The keys in it
// are public test fixtures. Refresh the copy when Omnia changes the file.
type vectorFile struct {
	Vectors []struct {
		HMACKey   string `json:"hmacKey"`
		Input     string `json:"input"`
		Pseudonym string `json:"pseudonym"`
	} `json:"vectors"`
	Fingerprints []struct {
		HMACKey     string `json:"hmacKey"`
		Fingerprint string `json:"fingerprint"`
	} `json:"fingerprints"`
	// WorkspaceKeys pin the per-workspace derivation. The seeder never derives
	// a workspace key (it reads the derived one from the workspace Secret), so
	// only the pseudonym and fingerprint under workspaceKey are checked.
	WorkspaceKeys []struct {
		WorkspaceKey string `json:"workspaceKey"`
		Input        string `json:"input"`
		Pseudonym    string `json:"pseudonym"`
		Fingerprint  string `json:"fingerprint"`
	} `json:"workspaceKeys"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/pseudonym_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var f vectorFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Vectors) == 0 || len(f.Fingerprints) == 0 || len(f.WorkspaceKeys) == 0 {
		t.Fatal("vector file has an empty section")
	}
	return f
}

func TestIDMatchesOmniaVectors(t *testing.T) {
	for _, v := range loadVectors(t).Vectors {
		if got := New(v.HMACKey).ID(v.Input); got != v.Pseudonym {
			t.Errorf("New(%q).ID(%q) = %q, want %q", v.HMACKey, v.Input, got, v.Pseudonym)
		}
	}
}

func TestFingerprintMatchesOmniaVectors(t *testing.T) {
	for _, v := range loadVectors(t).Fingerprints {
		if got := New(v.HMACKey).Fingerprint(); got != v.Fingerprint {
			t.Errorf("New(%q).Fingerprint() = %q, want %q", v.HMACKey, got, v.Fingerprint)
		}
	}
}

func TestWorkspaceKeyVectors(t *testing.T) {
	for _, v := range loadVectors(t).WorkspaceKeys {
		p := New(v.WorkspaceKey)
		if got := p.ID(v.Input); got != v.Pseudonym {
			t.Errorf("workspace key %q: ID(%q) = %q, want %q", v.WorkspaceKey, v.Input, got, v.Pseudonym)
		}
		if got := p.Fingerprint(); got != v.Fingerprint {
			t.Errorf("workspace key %q: Fingerprint() = %q, want %q", v.WorkspaceKey, got, v.Fingerprint)
		}
	}
}

func TestKeyed(t *testing.T) {
	if New("").Keyed() {
		t.Error("empty key reported as keyed")
	}
	if !New("k").Keyed() {
		t.Error("non-empty key reported as unkeyed")
	}
}
