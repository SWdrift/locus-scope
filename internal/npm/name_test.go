package npm

import "testing"

func TestPackageNameAndSpecParsing(t *testing.T) {
	validNames := []string{"plain", "name-with.dots", "@example/app"}
	for _, value := range validNames {
		if got, err := ParsePackageName(value); err != nil || got != value {
			t.Errorf("ParsePackageName(%q) = %q, %v", value, got, err)
		}
	}
	invalidNames := []string{"", "UPPER", ".hidden", "@scope", "@scope/name/subpath", "name@1", "name with space", "node_modules"}
	for _, value := range invalidNames {
		if _, err := ParsePackageName(value); err == nil {
			t.Errorf("ParsePackageName(%q) succeeded", value)
		}
	}

	tests := []struct {
		input      string
		name       string
		constraint string
	}{
		{input: "plain", name: "plain", constraint: "*"},
		{input: "plain@^1.2.0", name: "plain", constraint: "^1.2.0"},
		{input: "@example/app", name: "@example/app", constraint: "*"},
		{input: "@example/app@~2.1.0", name: "@example/app", constraint: "~2.1.0"},
	}
	for _, test := range tests {
		got, err := ParsePackageSpec(test.input)
		if err != nil {
			t.Fatalf("ParsePackageSpec(%q): %v", test.input, err)
		}
		if got.Name != test.name || got.Constraint != test.constraint {
			t.Errorf("ParsePackageSpec(%q) = %#v", test.input, got)
		}
	}
	for _, value := range []string{"plain@", "plain@latest", "@example/app@file:../app", "@example/app/subpath"} {
		if _, err := ParsePackageSpec(value); err == nil {
			t.Errorf("ParsePackageSpec(%q) succeeded", value)
		}
	}
}

func TestNPMVersionMatchingAndIdentity(t *testing.T) {
	got, err := HighestMatching([]string{"1.0.0", "2.0.0", "1.9.0", "1.10.0-beta.1"}, "^1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.9.0" {
		t.Fatalf("HighestMatching = %q, want 1.9.0", got)
	}
	identity, err := Identity("@example/app", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	name, version, err := ParseIdentity(identity)
	if err != nil || name != "@example/app" || version != "1.2.3" {
		t.Fatalf("ParseIdentity(%q) = %q, %q, %v", identity, name, version, err)
	}
	for _, value := range []string{"@example/app@1.2.3", "npm:@example/app@v1.2.3", "npm:@example/app@latest"} {
		if _, _, err := ParseIdentity(value); err == nil {
			t.Errorf("ParseIdentity(%q) succeeded", value)
		}
	}
}
