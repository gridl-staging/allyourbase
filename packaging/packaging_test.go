package packaging

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()

	content := readFile(t, path)
	manifest := map[string]any{}
	if err := json.Unmarshal([]byte(content), &manifest); err != nil {
		t.Fatalf("parse %s as json: %v", path, err)
	}

	return manifest
}

func readYAMLMap(t *testing.T, path string) map[string]any {
	t.Helper()

	content := readFile(t, path)
	manifest := map[string]any{}
	if err := yaml.Unmarshal([]byte(content), &manifest); err != nil {
		t.Fatalf("parse %s as yaml: %v", path, err)
	}

	return manifest
}

func requireMapKey(t *testing.T, m map[string]any, key string) any {
	t.Helper()

	value, ok := m[key]
	if !ok {
		t.Fatalf("missing required key %q", key)
	}

	return value
}

func requireString(t *testing.T, value any, label string) string {
	t.Helper()

	s, ok := value.(string)
	if !ok {
		t.Fatalf("%s is not a string", label)
	}

	return s
}

func requireStringEqual(t *testing.T, value any, label string, want string) {
	t.Helper()

	got := requireString(t, value, label)
	if got != want {
		t.Fatalf("%s = %q, want %q", label, got, want)
	}
}

func requireMap(t *testing.T, value any, label string) map[string]any {
	t.Helper()

	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s is not a map", label)
	}

	return m
}

func requireSlice(t *testing.T, value any, label string) []any {
	t.Helper()

	s, ok := value.([]any)
	if !ok {
		t.Fatalf("%s is not a list", label)
	}

	return s
}

func TestWingetManifestContainsRequiredKeys(t *testing.T) {
	manifest := readYAMLMap(t, "winget/allyourbase.ayb.yaml")

	requireStringEqual(t, requireMapKey(t, manifest, "PackageIdentifier"), "PackageIdentifier", "allyourbase.ayb")
	requireStringEqual(t, requireMapKey(t, manifest, "PackageVersion"), "PackageVersion", "0.0.0")
	requireStringEqual(t, requireMapKey(t, manifest, "PackageName"), "PackageName", "AYB")
	requireStringEqual(t, requireMapKey(t, manifest, "Publisher"), "Publisher", "Gridl")
	requireStringEqual(t, requireMapKey(t, manifest, "License"), "License", "MIT")
	requireStringEqual(t, requireMapKey(t, manifest, "ManifestVersion"), "ManifestVersion", "1.6.0")

	installers := requireSlice(t, requireMapKey(t, manifest, "Installers"), "Installers")
	if len(installers) != 2 {
		t.Fatalf("Installers length = %d, want 2", len(installers))
	}

	installerByArch := map[string]map[string]any{}
	for _, installerAny := range installers {
		installer := requireMap(t, installerAny, "installer entry")
		archAny := requireMapKey(t, installer, "Architecture")
		arch, ok := archAny.(string)
		if !ok {
			t.Fatalf("Architecture is not a string")
		}
		installerByArch[arch] = installer
	}

	for _, arch := range []string{"x64", "arm64"} {
		installer, ok := installerByArch[arch]
		if !ok {
			t.Fatalf("missing installer for arch %q", arch)
		}

		requireStringEqual(t, requireMapKey(t, installer, "InstallerType"), "InstallerType", "zip")
		requireStringEqual(t, requireMapKey(t, installer, "NestedInstallerType"), "NestedInstallerType", "portable")

		url := requireString(t, requireMapKey(t, installer, "InstallerUrl"), "InstallerUrl")
		switch arch {
		case "x64":
			if want := "https://github.com/gridlhq/allyourbase/releases/download/v0.0.0/ayb_0.0.0_windows_amd64.zip"; url != want {
				t.Fatalf("installer %s url = %q, want %q", arch, url, want)
			}
		case "arm64":
			if want := "https://github.com/gridlhq/allyourbase/releases/download/v0.0.0/ayb_0.0.0_windows_arm64.zip"; url != want {
				t.Fatalf("installer %s url = %q, want %q", arch, url, want)
			}
		}

		hash := requireString(t, requireMapKey(t, installer, "InstallerSha256"), "InstallerSha256")
		if !strings.HasPrefix(hash, "REPLACE_WITH_SHA256_") {
			t.Fatalf("installer %s sha placeholder %q does not use REPLACE_WITH_SHA256_ prefix", arch, hash)
		}

		nestedFiles := requireSlice(t, requireMapKey(t, installer, "NestedInstallerFiles"), "NestedInstallerFiles")
		if len(nestedFiles) == 0 {
			t.Fatalf("installer %s has no nested installer files", arch)
		}
		firstFile := requireMap(t, nestedFiles[0], "NestedInstallerFiles[0]")
		if rel := requireMapKey(t, firstFile, "RelativeFilePath"); rel != "ayb.exe" {
			t.Fatalf("installer %s nested file path = %v, want ayb.exe", arch, rel)
		}
	}
}

func TestScoopManifestContainsRequiredKeys(t *testing.T) {
	manifest := readJSONMap(t, "scoop/ayb.json")

	required := []string{"version", "architecture", "bin", "checkver", "autoupdate"}
	for _, key := range required {
		requireMapKey(t, manifest, key)
	}
	requireStringEqual(t, requireMapKey(t, manifest, "version"), "version", "0.0.0")
	requireStringEqual(t, requireMapKey(t, manifest, "bin"), "bin", "ayb.exe")
	requireStringEqual(t, requireMapKey(t, manifest, "homepage"), "homepage", "https://github.com/gridlhq/allyourbase")
	requireStringEqual(t, requireMapKey(t, manifest, "license"), "license", "MIT")
	requireStringEqual(t, requireMapKey(t, manifest, "description"), "description", "Backend-as-a-Service for PostgreSQL. Single binary, one config file.")

	architecture := requireMap(t, requireMapKey(t, manifest, "architecture"), "architecture")
	for _, arch := range []string{"64bit", "arm64"} {
		entry := requireMap(t, requireMapKey(t, architecture, arch), "architecture."+arch)
		url := requireString(t, requireMapKey(t, entry, "url"), "architecture."+arch+".url")
		hash := requireString(t, requireMapKey(t, entry, "hash"), "architecture."+arch+".hash")
		if !strings.HasPrefix(hash, "REPLACE_WITH_SHA256_") {
			t.Fatalf("architecture.%s hash placeholder %q does not use REPLACE_WITH_SHA256_ prefix", arch, hash)
		}
		switch arch {
		case "64bit":
			if want := "https://github.com/gridlhq/allyourbase/releases/download/v0.0.0/ayb_0.0.0_windows_amd64.zip"; url != want {
				t.Fatalf("architecture.%s url = %q, want %q", arch, url, want)
			}
		case "arm64":
			if want := "https://github.com/gridlhq/allyourbase/releases/download/v0.0.0/ayb_0.0.0_windows_arm64.zip"; url != want {
				t.Fatalf("architecture.%s url = %q, want %q", arch, url, want)
			}
		}
	}

	checkver := requireMap(t, requireMapKey(t, manifest, "checkver"), "checkver")
	requireStringEqual(t, requireMapKey(t, checkver, "github"), "checkver.github", "https://github.com/gridlhq/allyourbase")

	autoupdate := requireMap(t, requireMapKey(t, manifest, "autoupdate"), "autoupdate")
	autoArch := requireMap(t, requireMapKey(t, autoupdate, "architecture"), "autoupdate.architecture")
	requireStringEqual(
		t,
		requireMapKey(t, requireMap(t, requireMapKey(t, autoArch, "64bit"), "autoupdate.architecture.64bit"), "url"),
		"autoupdate.architecture.64bit.url",
		"https://github.com/gridlhq/allyourbase/releases/download/v$version/ayb_$version_windows_amd64.zip",
	)
	requireStringEqual(
		t,
		requireMapKey(t, requireMap(t, requireMapKey(t, autoArch, "arm64"), "autoupdate.architecture.arm64"), "url"),
		"autoupdate.architecture.arm64.url",
		"https://github.com/gridlhq/allyourbase/releases/download/v$version/ayb_$version_windows_arm64.zip",
	)
}

func TestNixDerivationContainsRequiredKeys(t *testing.T) {
	content := readFile(t, "nix/default.nix")

	required := []string{
		"buildGoModule",
		"pname",
		"version",
		"vendorHash",
		"meta",
	}
	for _, key := range required {
		if !strings.Contains(content, key) {
			t.Fatalf("nix derivation missing required token %q", key)
		}
	}
}

func TestGoreleaserContainsScoopBucketConfig(t *testing.T) {
	manifest := readYAMLMap(t, "../.goreleaser.yaml")

	scoops := requireSlice(t, requireMapKey(t, manifest, "scoops"), "scoops")
	if len(scoops) == 0 {
		t.Fatalf("scoops is empty")
	}
	firstScoop := requireMap(t, scoops[0], "scoops[0]")

	bucket := requireMap(t, requireMapKey(t, firstScoop, "bucket"), "scoops[0].bucket")

	if owner := requireMapKey(t, bucket, "owner"); owner != "gridlhq" {
		t.Fatalf("scoop.bucket.owner = %v, want gridlhq", owner)
	}
	if name := requireMapKey(t, bucket, "name"); name != "scoop-bucket" {
		t.Fatalf("scoop.bucket.name = %v, want scoop-bucket", name)
	}

	requireStringEqual(t, requireMapKey(t, firstScoop, "homepage"), "scoops[0].homepage", "https://github.com/gridlhq/allyourbase")
	requireStringEqual(
		t,
		requireMapKey(t, firstScoop, "description"),
		"scoops[0].description",
		"Backend-as-a-Service for PostgreSQL. Single binary, one config file.",
	)
	requireStringEqual(t, requireMapKey(t, firstScoop, "license"), "scoops[0].license", "MIT")
}
