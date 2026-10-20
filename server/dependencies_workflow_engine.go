package server

import (
	"context"
	"fmt"
	"strings"
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
	// Modes resolves each caller class's mode from live config.
	Modes *session.FlagClassModeSource
	// Recorder persists the shadow ledger; its Run is started with the server.
	Recorder *session.ShadowLedgerRecorder
	// Soak backs /actuator/workflow-engine.
	Soak *WorkflowEngineSoakSource
	// Status backs GetWorkflowEngineStatus; GateWired is filled by InjectTransitionGate.
	Status *services.WorkflowEngineStatusSource
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
	modes := services.NewWorkflowEngineClassModeSource(loadConfig, nil)
	engine := session.NewSwitchableWorkflowEngine(def, cfgEngine, modes,
		session.WithShadowRecorder(recorder),
		session.WithConfigLabeler(session.NewConfigLabeler(stageFingerprintFunc(entClient), nil)),
	)

	soak := &WorkflowEngineSoakSource{
		Ledger: func() (session.ShadowLedger, error) { return recorder.Snapshot(), nil },
		AnyClassEnforced: func() bool {
			cfg := loadConfig()
			return config.EffectiveWorkflowEngineEnforce(cfg) || config.EffectiveWorkflowEngineEnforceAgent(cfg)
		},
	}
	if dir != "" {
		soak.ReproPath = session.ShadowLedgerPath(dir)
	}
	if entClient != nil && cfgEngine != nil {
		soak.ConfigDiff = NewWorkflowEngineConfigDiff(entClient, cfgEngine)
	}

	status := &services.WorkflowEngineStatusSource{
		ModeFor:          modes.ModeFor,
		Ledger:           soak.Ledger,
		ConfigDiff:       soak.ConfigDiff,
		InvalidatorWired: func() bool { return cfgEngine != nil },
		FallbackActive:   func() bool { return anyFallback(engine) },
	}

	logWorkflowEngineStartup(modes.Mode(), cfgEngine)
	return &WorkflowEngineWiring{Engine: engine, Modes: modes, Recorder: recorder, Soak: soak, Status: status}
}

// InjectTransitionGate wires the selector and the denial reporter into storage
// in one call, so the gate can never be live without its reporter, and logs the
// startup line that says so.
func (w *WorkflowEngineWiring) InjectTransitionGate(storage *session.Storage, reporter session.TransitionDeniedReporter) error {
	if err := storage.SetTransitionGate(w.Engine, reporter); err != nil {
		return fmt.Errorf("inject transition gate: %w", err)
	}
	w.Status.GateWired = storage.TransitionGateWired
	log.Info(transitionGateStartupLine(storage, reporter != nil, w.Modes))
	return nil
}

// anyFallback reports whether the enforce composition has ever answered from
// Default because the Configured engine was unusable.
func anyFallback(e *session.SwitchableWorkflowEngine) bool {
	for _, r := range []session.FallbackReason{session.FallbackCfgNil, session.FallbackCacheEmpty, session.FallbackLoadError} {
		if e.FallbackTotal(r) > 0 {
			return true
		}
	}
	return false
}

// transitionGateStartupLine is the line an operator greps for to confirm the
// choke point is live. The system class has no enforce flag until Task 1.5.8g,
// so it reads shadow (or off when the shadow flag is off).
func transitionGateStartupLine(storage *session.Storage, reporterWired bool, modes session.ClassModeSource) string {
	wired := func(b bool) string {
		if b {
			return "wired"
		}
		return "unwired"
	}
	classModes := make([]string, 0, 3)
	for _, c := range []session.CallerClass{session.ClassOperator, session.ClassAgent, session.ClassSystem} {
		classModes = append(classModes, fmt.Sprintf("%s:%s", c, engineModeName(modes.ModeFor(c))))
	}
	return fmt.Sprintf("workflow engine gate: transition_gate=%s reporter=%s class_modes=%s",
		wired(storage.TransitionGateWired()), wired(reporterWired), strings.Join(classModes, ","))
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
