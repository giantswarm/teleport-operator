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

func TestGetConfigmapDataFromTemplate_NestedWhenNewer(t *testing.T) {
	data := GetConfigmapDataFromTemplate("tok", "proxy:443", "kube", "16.0.0", []string{"kube", "app"}, "0.11.0")
	if want := "teleport-kube-agent:\n"; data[:len(want)] != want {
		t.Fatalf("expected nested layout, got:\n%s", data)
	}
}

func TestGetConfigmapDataFromTemplate_FlatWhenOlder(t *testing.T) {
	data := GetConfigmapDataFromTemplate("tok", "proxy:443", "kube", "16.0.0", []string{"kube", "app"}, "0.10.8")
	if want := "roles:"; data[:len(want)] != want {
		t.Fatalf("expected flat layout, got:\n%s", data)
	}
}

func TestResolveTeleportVersionOverride(t *testing.T) {
	cases := []struct {
		name            string
		appVersion      string
		teleportVersion string
		want            string
	}{
		{"empty teleport version", "0.11.0", "", ""},
		{"flat layout passes value through", "0.10.8", "1.0.0", "1.0.0"},
		{"flat layout passes unparseable through", "0.10.8", "master-abc", "master-abc"},
		{"nested at bundled is kept", "0.11.0", "18.7.6", "18.7.6"},
		{"nested above bundled is kept", "0.11.0", "18.8.0", "18.8.0"},
		{"nested below bundled is dropped", "0.11.0", "18.7.5", ""},
		{"nested far below bundled is dropped", "0.11.0", "1.0.0", ""},
		{"nested with v-prefix kept", "0.11.0", "v18.7.6", "v18.7.6"},
		{"nested unparseable is dropped", "0.11.0", "master-abc", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveTeleportVersionOverride(c.appVersion, c.teleportVersion); got != c.want {
				t.Fatalf("ResolveTeleportVersionOverride(%q, %q) = %q, want %q", c.appVersion, c.teleportVersion, got, c.want)
			}
		})
	}
}
