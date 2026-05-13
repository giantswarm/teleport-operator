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
		{"0.10.9", true},
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
