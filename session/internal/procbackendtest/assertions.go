package procbackendtest

import (
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/tymux"
)

var (
	_ tmux.TmuxManager        = (*MockTmuxManager)(nil)
	_ instance.ProcessManager = (*FakeLivenessProcessManager)(nil)
	_ instance.ProcessManager = (*FakeSendKeysProcessManager)(nil)
	_ tymux.TymuxManager      = (*FakeTymuxManagerForStartRestore)(nil)
	_ tymux.TymuxManager      = (*FakeTymuxManagerForBackendRestarted)(nil)
	_ tymux.TymuxManager      = (*FakeTymuxManagerForSetCommand)(nil)
)
