package backend_test

import (
	"testing"

	"github.com/chrisbanes/grove/internal/backend"
)

func TestForName_ValidBackends(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"cp", "image"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			impl, err := backend.ForName(name)
			if err != nil {
				t.Fatalf("ForName(%q) error = %v", name, err)
			}
			if impl == nil {
				t.Fatalf("ForName(%q) returned nil implementation", name)
			}
		})
	}
}

func TestForName_InvalidBackend(t *testing.T) {
	t.Parallel()

	impl, err := backend.ForName("nope")
	if err == nil {
		t.Fatalf("ForName() expected error for invalid backend, got impl=%T", impl)
	}
}
