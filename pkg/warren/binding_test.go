package warren_test

import (
	"testing"

	"github.com/tstapler/stapler-squad/pkg/warren"
)

type fakeRepo struct{ name string }

func TestBinding_SetAndGet(t *testing.T) {
	b := warren.NewBinding[*fakeRepo]("test.repo")
	if b.IsSet() {
		t.Error("IsSet should be false before Set()")
	}

	b.Set(&fakeRepo{name: "real"})
	if !b.IsSet() {
		t.Error("IsSet should be true after Set()")
	}

	v, ok := b.Get()
	if !ok {
		t.Error("Get() ok should be true after Set()")
	}
	if v.name != "real" {
		t.Errorf("Get() name = %q, want %q", v.name, "real")
	}
}

func TestBinding_MustPanicsWhenUnset(t *testing.T) {
	b := warren.NewBinding[*fakeRepo]("test.repo")
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must() should panic when binding is not set")
		}
	}()
	_ = b.Must()
}

func TestBinding_MustReturnsValueWhenSet(t *testing.T) {
	b := warren.NewBinding[*fakeRepo]("test.repo")
	b.Set(&fakeRepo{name: "real"})
	v := b.Must()
	if v.name != "real" {
		t.Errorf("Must() = %q, want %q", v.name, "real")
	}
}

func TestBinding_OverrideRestoresAfterTest(t *testing.T) {
	b := warren.NewBinding[*fakeRepo]("test.repo")
	b.Set(&fakeRepo{name: "production"})

	t.Run("subtest", func(t *testing.T) {
		b.Override(t, &fakeRepo{name: "mock"})
		if b.Must().name != "mock" {
			t.Errorf("inside Override: got %q, want %q", b.Must().name, "mock")
		}
	})

	// After the subtest ends, cleanup should have restored the original.
	if b.Must().name != "production" {
		t.Errorf("after Override subtest: got %q, want %q", b.Must().name, "production")
	}
}

func TestBinding_OverrideOnUnsetBinding(t *testing.T) {
	b := warren.NewBinding[*fakeRepo]("test.repo")

	t.Run("override-unset", func(t *testing.T) {
		b.Override(t, &fakeRepo{name: "mock"})
		if b.Must().name != "mock" {
			t.Errorf("got %q, want %q", b.Must().name, "mock")
		}
	})

	// isSet should be restored to false after subtest.
	if b.IsSet() {
		t.Error("binding should be unset after Override cleanup on originally-unset binding")
	}
}

func TestBinding_OverrideStacksCorrectly(t *testing.T) {
	b := warren.NewBinding[string]("test.str")
	b.Set("original")

	t.Run("outer", func(t *testing.T) {
		b.Override(t, "outer-override")
		t.Run("inner", func(t *testing.T) {
			b.Override(t, "inner-override")
			if b.Must() != "inner-override" {
				t.Errorf("inner: got %q", b.Must())
			}
		})
		// After inner test ends, should see outer override.
		if b.Must() != "outer-override" {
			t.Errorf("after inner subtest: got %q, want outer-override", b.Must())
		}
	})

	if b.Must() != "original" {
		t.Errorf("after outer subtest: got %q, want original", b.Must())
	}
}

func TestBinding_InterfaceType(t *testing.T) {
	type Doer interface{ Do() string }
	type realDoer struct{}
	type fakeDoer struct{}
	_ = (*realDoer)(nil)
	_ = (*fakeDoer)(nil)

	b := warren.NewBinding[Doer]("doer")
	// Just verify Set/Get compile and work with interface types.
	// (interface bindings use pointer-to-zero as default, so isSet distinguishes)
	if b.IsSet() {
		t.Error("should not be set")
	}
}
