// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestTextLike(t *testing.T) {
	for _, name := range []string{"guide.md", "data.JSON", "notes.log", "records.csv"} {
		if !textLike(name) {
			t.Errorf("textLike(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"photo.png", "archive.zip", "readme"} {
		if textLike(name) {
			t.Errorf("textLike(%q) = true, want false", name)
		}
	}
}

func TestLoadConfigRequiresCoreSettings(t *testing.T) {
	for _, name := range []string{"MEMORY_API_URL", "WORKSPACE_ID", "S3_BUCKET"} {
		t.Setenv(name, "")
	}
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() succeeded without required settings")
	}
}

func TestHiddenSkipsDotSegments(t *testing.T) {
	for key, want := range map[string]bool{
		"switching.md": false, "manuals/switching.md": false,
		"..data/switching.md": true, "..2026_11_10_10_54_16.553700652/switching.md": true, ".hidden.md": true,
	} {
		if got := hidden(key); got != want {
			t.Errorf("hidden(%q) = %v, want %v", key, got, want)
		}
	}
}
