package modlibrary

import (
	"strings"
	"testing"
)

// An identity-only expectation requires authored metadata and refuses an id
// or version mismatch before staging. It never bypasses ZIP config parsing
// (DESIGN_MODS_MUTATORS §5.3).
func TestExpectIdentityChecksPackage(t *testing.T) {
	config := `{"schema":2,"id":"sample","name":"Sample","version":"1.0","rules":{"minimumGameplay":"community-3.9"},"keys":{"profile":"community"}}`
	archive := writeZip(t, t.TempDir(), "sample.zip", zipItem{name: MetadataFile, body: config}, zipItem{name: "a.ufo", body: "a"})
	for _, tc := range []struct {
		name string
		want ExpectedIdentity
	}{
		{"id", ExpectedIdentity{ID: "other", Version: "1.0"}},
		{"version", ExpectedIdentity{ID: "sample", Version: "2.0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lib := openTestLibrary(t)
			if _, err := lib.InstallArchive(archive, InstallOptions{ExpectIdentity: &tc.want}); err == nil || !strings.Contains(err.Error(), "disagrees with the catalogue on "+tc.name) {
				t.Fatalf("install = %v, want an identity disagreement", err)
			}
			assertNothingInstalled(t, lib)
		})
	}
	identity := ExpectedIdentity{ID: "sample", Version: "1.0"}
	for _, tc := range []struct {
		name, metadata, refusal string
	}{
		{"missing metadata", "", "no metadata"},
		{"unknown schema", `{"schema":3,"id":"sample","name":"Sample","version":"1.0"}`, "not supported"},
		{"invalid config", `{"schema":2,"id":"sample","name":"Sample","version":"1.0","rules":{"minimumGameplay":"fast"}}`, "minimumGameplay"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := []zipItem{{name: "a.ufo", body: "a"}}
			if tc.metadata != "" {
				items = append(items, zipItem{name: MetadataFile, body: tc.metadata})
			}
			bad := writeZip(t, t.TempDir(), "bad.zip", items...)
			lib := openTestLibrary(t)
			if _, err := lib.InstallArchive(bad, InstallOptions{ExpectIdentity: &identity}); err == nil || !strings.Contains(err.Error(), tc.refusal) {
				t.Fatalf("install = %v, want %s refused", err, tc.name)
			}
			assertNothingInstalled(t, lib)
		})
	}
	for _, metadata := range []string{metadataJSON(t, sampleMetadata()), config} {
		compatible := writeZip(t, t.TempDir(), "compatible.zip", zipItem{name: MetadataFile, body: metadata}, zipItem{name: "a.ufo", body: "a"})
		if _, err := openTestLibrary(t).InstallArchive(compatible, InstallOptions{ExpectIdentity: &identity}); err != nil {
			t.Fatalf("matching package = %v", err)
		}
	}
	// Supplying both expectations retains the strict expectation's checks.
	strict := Metadata{Schema: 1, ID: identity.ID, Name: "Sample", Version: identity.Version}
	lib := openTestLibrary(t)
	if _, err := lib.InstallArchive(archive, InstallOptions{ExpectIdentity: &identity, Expect: &strict}); err == nil || !strings.Contains(err.Error(), "disagrees with the catalogue on schema") {
		t.Fatalf("strict expectation = %v, want a schema disagreement", err)
	}
	assertNothingInstalled(t, lib)
}
