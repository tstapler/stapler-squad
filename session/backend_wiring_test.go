package session

import (
	"reflect"
	"testing"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/procbackend"
)

func TestProcessManagerHook_RegisteredWhenSessionLinked(t *testing.T) {
	t.Parallel()
	if instance.ProcessManagerFactoryHook == nil {
		t.Fatal("instance.ProcessManagerFactoryHook is nil: session/backend_wiring.go must set it to procbackend.NewProcessManager")
	}
	got := reflect.ValueOf(instance.ProcessManagerFactoryHook).Pointer()
	want := reflect.ValueOf(procbackend.NewProcessManager).Pointer()
	if got != want {
		t.Error("instance.ProcessManagerFactoryHook is not procbackend.NewProcessManager")
	}
}
