package server

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// WorkflowEngineWiring is the production workflow-engine stack: the selector
// that BacklogService, the SyncLoops and the Storage gate consult, the shadow
// recorder it feeds, and the actuator's read side of that recorder.
type WorkflowEngineWiring struct {
	// Engine is the only engine handed to BacklogService and SyncLoop. It must
	// never be handed to BacklogLifecycleListener, which needs the concrete
	// *session.ConfiguredWorkflowEngine (see BacklogLifecycleListener.workflowEngine).
	Engine *session.SwitchableWorkflowEngine
	// Recorder persists the shadow ledger; its Run is started with the server.
	Recorder *session.ShadowLedgerRecorder
	// Soak backs /actuator/workflow-engine.
	Soak *WorkflowEngineSoakSource
}

// newWorkflowEngineWiring builds the selector over def and cfgEngine (nil
// when stage-config construction failed; the selector then answers from Default)
// with the shadow recorder in place before any engine call can happen.
func newWorkflowEngineWiring(def *session.DefaultWorkflowEngine, cfgEngine *session.ConfiguredWorkflowEngine, entClient *ent.Client, loadConfig func() *config.Config) *WorkflowEngineWiring {
	dir, err := session.ShadowLedgerDir()
	if err != nil {
		log.Warn("workflow engine shadow ledger directory unavailable; the soak window will not persist", "err", err)
		dir = ""
	}
	recorder := session.NewShadowLedgerRecorder(dir)
	modes := services.NewWorkflowEngineModeSource(loadConfig, nil)
	engine := session.NewSwitchableWorkflowEngine(def, cfgEngine, modes,
		session.WithShadowRecorder(recorder),
		session.WithConfigLabeler(session.NewConfigLabeler(stageFingerprintFunc(entClient), nil)),
	)

	soak := &WorkflowEngineSoakSource{
		Ledger:           func() (session.ShadowLedger, error) { return recorder.Snapshot(), nil },
		AnyClassEnforced: func() bool { return config.EffectiveWorkflowEngineEnforce(loadConfig()) },
	}
	if dir != "" {
		soak.ReproPath = session.ShadowLedgerPath(dir)
	}
	if entClient != nil && cfgEngine != nil {
		soak.ConfigDiff = NewWorkflowEngineConfigDiff(entClient, cfgEngine)
	}

	logWorkflowEngineStartup(modes.Mode(), cfgEngine)
	return &WorkflowEngineWiring{Engine: engine, Recorder: recorder, Soak: soak}
}

// RegisterStatusDetail shows the shadow divergence count on the shadow flag's status line.
func (w *WorkflowEngineWiring) RegisterStatusDetail(svc *services.SessionService) {
	svc.SetStatusDetailProvider(config.WorkflowEngineShadowFeatureFlag,
		services.WorkflowEngineShadowStatusDetail(w.Recorder.Snapshot, time.Now))
}

func stageFingerprintFunc(entClient *ent.Client) func() (string, error) {
	return func() (string, error) {
		if entClient == nil {
			return "", fmt.Errorf("no ent client")
		}
		return session.StageConfigFingerprint(context.Background(), entClient)
	}
}

// logWorkflowEngineStartup logs the one line an operator greps for. A nil
// cfgEngine means stage/transition/gate CRUD writes cannot invalidate any
// cache, so that case is a WARN.
func logWorkflowEngineStartup(mode session.EngineMode, cfgEngine *session.ConfiguredWorkflowEngine) {
	msg := fmt.Sprintf("workflow engine: mode=%s configured_loaded=%t invalidator_wired=%t",
		engineModeName(mode), cfgEngine != nil && cfgEngine.HasAnyStage(), cfgEngine != nil)
	if cfgEngine == nil {
		log.Warn(msg)
		return
	}
	log.Info(msg)
}

func engineModeName(m session.EngineMode) string {
	switch m {
	case session.ModeOff:
		return "off"
	case session.ModeShadow:
		return "shadow"
	case session.ModeEnforce:
		return "enforce"
	}
	return "unknown"
}
