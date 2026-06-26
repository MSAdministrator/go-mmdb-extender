package source

import (
	"errors"
	"testing"

	"github.com/maxmind/mmdbwriter"
)

// stubSource is a minimal Source for registry tests.
type stubSource struct{ name string }

func (s stubSource) Name() string                  { return s.name }
func (s stubSource) Enrich(*mmdbwriter.Tree) error { return nil }

// resetRegistry clears the package-level registry so each test starts clean.
func resetRegistry(t *testing.T) {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	factories = map[string]Factory{}
}

func TestRegisterAndRegistered(t *testing.T) {
	resetRegistry(t)

	Register("bbb", func(map[string]any) (Source, error) { return stubSource{"bbb"}, nil })
	Register("aaa", func(map[string]any) (Source, error) { return stubSource{"aaa"}, nil })

	got := Registered()
	if len(got) != 2 || got[0] != "aaa" || got[1] != "bbb" {
		t.Errorf("Registered() = %v, want [aaa bbb] (sorted)", got)
	}
}

func TestRegister_DuplicatePanics(t *testing.T) {
	resetRegistry(t)
	Register("dup", func(map[string]any) (Source, error) { return nil, nil })

	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	Register("dup", func(map[string]any) (Source, error) { return nil, nil })
}

func TestBuild_SortedOrder(t *testing.T) {
	resetRegistry(t)
	Register("zeta", func(map[string]any) (Source, error) { return stubSource{"zeta"}, nil })
	Register("alpha", func(map[string]any) (Source, error) { return stubSource{"alpha"}, nil })

	sources, err := Build(map[string]map[string]any{
		"zeta":  {},
		"alpha": {},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("len = %d, want 2", len(sources))
	}
	if sources[0].Name() != "alpha" || sources[1].Name() != "zeta" {
		t.Errorf("order = [%s %s], want [alpha zeta]", sources[0].Name(), sources[1].Name())
	}
}

func TestBuild_UnknownSource(t *testing.T) {
	resetRegistry(t)
	if _, err := Build(map[string]map[string]any{"ghost": {}}); err == nil {
		t.Fatal("expected error for unknown source, got nil")
	}
}

func TestBuild_FactoryError(t *testing.T) {
	resetRegistry(t)
	Register("boom", func(map[string]any) (Source, error) { return nil, errors.New("bad config") })

	if _, err := Build(map[string]map[string]any{"boom": {}}); err == nil {
		t.Fatal("expected factory error to propagate, got nil")
	}
}

func TestBuild_NilSourceSkipped(t *testing.T) {
	resetRegistry(t)
	// A factory returning (nil, nil) means "present but disabled" and should
	// be omitted from the built slice rather than included as a nil entry.
	Register("disabled", func(map[string]any) (Source, error) { return nil, nil })
	Register("enabled", func(map[string]any) (Source, error) { return stubSource{"enabled"}, nil })

	sources, err := Build(map[string]map[string]any{
		"disabled": {},
		"enabled":  {},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(sources) != 1 || sources[0].Name() != "enabled" {
		t.Errorf("got %d sources, want only [enabled]", len(sources))
	}
}
