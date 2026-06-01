package key

import "testing"

func TestUsesNestedKubeAgentValues(t *testing.T) {
	cases := []struct {
		version string
		nested  bool
	}{
		{"", false},
		{"not-a-version", false},
		{"0.3.0", false},
		{"0.10.8", false},
		{"v0.10.8", false},
		{"0.10.9", false},
		{"0.11.0", true},
		{"v0.11.0", true},
		{"1.0.0", true},
	}
	for _, c := range cases {
		t.Run(c.version, func(t *testing.T) {
			if got := UsesNestedKubeAgentValues(c.version); got != c.nested {
				t.Fatalf("UsesNestedKubeAgentValues(%q) = %v, want %v", c.version, got, c.nested)
			}
		})
	}
}

func TestGetConfigmapDataFromTemplate_NestedOnlyAtOrAbove0_11_0(t *testing.T) {
	data := GetConfigmapDataFromTemplate("tok", "proxy:443", "kube", "18.7.6", []string{"kube", "app"}, "0.11.0")
	if want := "teleport-kube-agent:\n"; data[:len(want)] != want {
		t.Fatalf("expected nested-only layout, got:\n%s", data)
	}
}

func TestGetConfigmapDataFromTemplate_DualBlockBelow0_11_0(t *testing.T) {
	cases := []string{"", "0.10.8", "not-a-version"}
	for _, tkaVersion := range cases {
		t.Run(tkaVersion, func(t *testing.T) {
			data := GetConfigmapDataFromTemplate("tok", "proxy:443", "kube", "18.7.6", []string{"kube", "app"}, tkaVersion)
			if !startsWith(data, "roles:") {
				t.Fatalf("expected flat root keys first, got:\n%s", data)
			}
			if !containsLine(data, "teleport-kube-agent:") {
				t.Fatalf("expected nested block, got:\n%s", data)
			}
		})
	}
}

func TestGetConfigmapDataFromTemplate_NestedFloorDropsDowngrade(t *testing.T) {
	data := GetConfigmapDataFromTemplate("tok", "proxy:443", "kube", "17.5.4", []string{"kube"}, "0.11.0")
	if containsLine(data, `  teleportVersionOverride: "17.5.4"`) {
		t.Fatalf("expected nested block to omit downgrade override, got:\n%s", data)
	}
}

func TestGetConfigmapDataFromTemplate_DualBlockFlatPassesOverride(t *testing.T) {
	data := GetConfigmapDataFromTemplate("tok", "proxy:443", "kube", "1.0.0", []string{"kube"}, "")
	if !containsLine(data, `teleportVersionOverride: "1.0.0"`) {
		t.Fatalf("expected flat block to keep passthrough override, got:\n%s", data)
	}
	if containsLine(data, `  teleportVersionOverride: "1.0.0"`) {
		t.Fatalf("expected nested block to drop below-floor override, got:\n%s", data)
	}
}

func TestResolveNestedTeleportVersionOverride(t *testing.T) {
	cases := []struct {
		name            string
		teleportVersion string
		want            string
	}{
		{"empty", "", ""},
		{"at bundled is kept", "18.7.6", "18.7.6"},
		{"above bundled is kept", "18.8.0", "18.8.0"},
		{"below bundled is dropped", "18.7.5", ""},
		{"far below bundled is dropped", "1.0.0", ""},
		{"with v-prefix is kept", "v18.7.6", "v18.7.6"},
		{"unparseable is dropped", "master-abc", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveNestedTeleportVersionOverride(c.teleportVersion); got != c.want {
				t.Fatalf("ResolveNestedTeleportVersionOverride(%q) = %q, want %q", c.teleportVersion, got, c.want)
			}
		})
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func containsLine(s, line string) bool {
	for _, l := range splitLines(s) {
		if l == line {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
