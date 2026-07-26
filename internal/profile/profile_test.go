package profile

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	for _, name := range []string{"default", "work-2", "a.b_c", strings.Repeat("a", 64)} {
		if err := Validate(name); err != nil {
			t.Errorf("Validate(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", "../escape", "space name", "-leading", strings.Repeat("a", 65)} {
		if err := Validate(name); err == nil {
			t.Errorf("Validate(%q) succeeded", name)
		}
	}
}
