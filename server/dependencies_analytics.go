package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	githubpkg "github.com/tstapler/stapler-squad/github"

	"entgo.io/ent/dialect"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/analytics"
	"github.com/tstapler/stapler-squad/server/analytics/escapedb"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/anthropicwire"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/server/services/insights"
	"github.com/tstapler/stapler-squad/server/services/sessionbundle"
	"github.com/tstapler/stapler-squad/server/services/unfinishedwork"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/artifacts"
	"github.com/tstapler/stapler-squad/session/contexthistory"
	"github.com/tstapler/stapler-squad/session/dbpool"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/importer"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/prpoller"
	"github.com/tstapler/stapler-squad/session/scanqueue"
	"github.com/tstapler/stapler-squad/session/scanstate"
	"github.com/tstapler/stapler-squad/session/search"
	"github.com/tstapler/stapler-squad/session/summaries"
	"github.com/tstapler/stapler-squad/session/tokens"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// unfinishedWiring is the unfinished-work scanner stack; every field is nil when the
// state store cannot be initialized.
type unfinishedWiring struct {
	scanner         *unfinished.Scanner
	stateStore      *unfinished.StateStore
	workSvc         *unfinishedwork.UnfinishedWorkService
	watchDirWatcher *unfinished.WatchDirWatcher
	prPoller        *prpoller.WorktreePRPoller
}

// buildUnfinishedWork initializes the unfinished-work scanner, state store and the
// worktree PR poller that shares the PR status poller's ETag cache.
func buildUnfinishedWork(svc *ServiceDeps, cfg *config.Config) unfinishedWiring {
	storage, eventBus := svc.Storage, svc.EventBus
	// Initialize UnfinishedWork scanner and state store.
	var (
		unfinishedScanner         *unfinished.Scanner
		unfinishedStateStore      *unfinished.StateStore
		unfinishedWorkSvc         *unfinishedwork.UnfinishedWorkService
		unfinishedWatchDirWatcher *unfinished.WatchDirWatcher
		worktreePRPoller          *prpoller.WorktreePRPoller
	)
	if configDir, configErr := config.GetConfigDir(); configErr == nil {
		statePath := filepath.Join(configDir, "unfinished_state.json")
		unfinishedStateStore, _ = unfinished.NewStateStore(statePath)
		if unfinishedStateStore != nil {
			var unfinishedFlags unfinished.Flags // all off without a config
			if cfg != nil {
				unfinishedFlags = unfinished.FlagsFromConfig(cfg.UnfinishedCaches) // restart-only, read once
			}
			unfinishedScanner = unfinished.NewScanner(eventBus, unfinishedStateStore, unfinished.WithFlags(unfinishedFlags))
			// The shadow-verify sampler arms itself at its default rate when any rollback
			// unit is on (Scanner.shadowEvery); the env var overrides it. The startup line
			// reports the effective rate.
			applyShadowVerifyEnv(unfinishedScanner, os.Getenv)
			unfinished.LogStartup(unfinishedScanner)
			unfinishedWatchDirWatcher = unfinished.NewWatchDirWatcher(unfinishedScanner, unfinishedStateStore)
			unfinishedWorkSvc = unfinishedwork.NewUnfinishedWorkService(unfinishedScanner, unfinishedStateStore, eventBus, storage, unfinishedWatchDirWatcher)
			log.Info("UnfinishedWorkService initialized", "state", statePath)
			if err := unfinished.RegisterMetrics(unfinishedScanner); err != nil {
				log.Warn("failed to register unfinished OTel metrics", "err", err)
			}

			// WorktreePRPoller enriches worktrees-without-sessions with GitHub PR data.
			// The scannerSource adapter bridges session/unfinished → session without a cycle.
			// Shares svc.PRStatusPoller's ETagCache instead of constructing its own — per
			// ADR-022, a separate cache would double API call volume for repos both pollers hit.
			worktreePRPoller = prpoller.NewWorktreePRPoller(
				svc.PRStatusPoller.ETagCache(),
				svc.PRStatusPoller,
			)
			worktreePRPoller.SetSource(&scannerSource{s: unfinishedScanner})
			worktreePRPoller.SetOnUpdated(func(repoPath, branch string, info *githubpkg.PRInfo) {
				log.Info("worktree PR updated", "repo", repoPath, "branch", branch, "pr", info.Number)
			})
		}
	} else {
		log.Warn("could not initialize UnfinishedWork state store", "err", configErr)
	}

	return unfinishedWiring{
		scanner:         unfinishedScanner,
		stateStore:      unfinishedStateStore,
		workSvc:         unfinishedWorkSvc,
		watchDirWatcher: unfinishedWatchDirWatcher,
		prPoller:        worktreePRPoller,
	}
}

// buildGitHubUserService builds the user PR cache and its service.
func buildGitHubUserService(svc *ServiceDeps, cfg *config.Config, unfinishedScanner *unfinished.Scanner) (*githubpkg.UserPRCache, *services.GitHubUserService) {
	storage, sessionService := svc.Storage, svc.SessionService
	// UserPRCache fetches all open PRs authored by the authenticated GitHub user.
	userPRCache := githubpkg.NewUserPRCacheWithConfig(userPRCacheConfigFromEnv())
	// The cache annotates each poll before it publishes, so streams never carry
	// PRs with missing session links.
	userPRCache.SetAnnotationSource(func() ([]githubpkg.PRAnnotationSession, []githubpkg.PRAnnotationWorktree) {
		return buildPRAnnotations(svc.PRStatusPoller, unfinishedScanner)
	})
	githubUserSvc := services.NewGitHubUserService(userPRCache, cfg.GetGitHubEnterpriseHosts())
	githubUserSvc.SetPRNudger(sessionService)
	if storage != nil {
		githubUserSvc.SetNudgeRecorder(storage)
	}
	githubUserSvc.SetPRDetailFetcher(githubpkg.GraphQLPRDetailFetcher{})
	githubUserSvc.SetPRTokenResolver(userPRCache)
	sessionService.SetUserPRCache(userPRCache)

	return userPRCache, githubUserSvc
}

// analyticsWiring holds the analytics and escape-event databases; each is nil when it
// failed to open.
type analyticsWiring struct {
	client    *ent.Client
	driver    dialect.ExecQuerier
	escClient *ent.Client
	escDriver dialect.ExecQuerier
	escDB     *dbpool.Database
}

// openAnalyticsStores opens the dedicated analytics and escape-event databases (non-fatal)
// and installs the files telemetry on the file service.
func openAnalyticsStores(sessionService *services.SessionService) analyticsWiring {
	// Open the dedicated analytics database (non-fatal: fall back gracefully on failure).
	var analyticsClient *ent.Client
	var analyticsDriver dialect.ExecQuerier
	var escapeEventsClient *ent.Client
	var escapeEventsDriver dialect.ExecQuerier
	var escapeEventsDB *dbpool.Database
	if configDir, configErr := config.GetConfigDir(); configErr == nil {
		ctx := context.Background()
		if ec, edb, ecErr := escapedb.Open(ctx, configDir); ecErr != nil {
			log.Warn("could not open escape events DB (escape capture disabled)", "err", ecErr)
		} else {
			escapeEventsClient, escapeEventsDriver, escapeEventsDB = ec, edb.Driver(), edb
			log.Info("escape events DB opened", "path", escapedb.Path(configDir))
		}
		if ac, drv, acErr := analytics.OpenAnalyticsDBWithDriver(ctx, configDir); acErr != nil {
			log.Warn("could not open analytics DB (will use log-only fallback)", "err", acErr)
		} else {
			analyticsClient = ac
			analyticsDriver = drv
			log.Info("analytics DB opened", "path", configDir+"/analytics.db")
		}
	} else {
		log.Warn("could not determine config dir for analytics DB", "err", configErr)
	}

	var filesAnalytics analytics.AnalyticsProvider = analytics.NewLogAnalyticsProvider()
	if analyticsClient != nil {
		filesAnalytics = analytics.NewSQLiteAnalyticsProvider(analyticsClient)
	}
	sessionService.GetFileService().SetTelemetry(services.NewFilesTelemetry(filesAnalytics))

	return analyticsWiring{
		client:    analyticsClient,
		driver:    analyticsDriver,
		escClient: escapeEventsClient,
		escDriver: escapeEventsDriver,
		escDB:     escapeEventsDB,
	}
}

// startDiagnosticsRetention prunes old diagnostics bundles at startup and daily.
func startDiagnosticsRetention() {
	// Diagnostics bundle retention: prune bundles older than 30 days at startup
	// and then once per day. Non-fatal: a missing directory is silently skipped.
	if retentionConfigDir, retentionConfigErr := config.GetConfigDir(); retentionConfigErr == nil {
		diagnosticsDir := filepath.Join(retentionConfigDir, "diagnostics")
		go func() {
			if err := sessionbundle.RunDiagnosticsRetention(diagnosticsDir, 30*24*time.Hour); err != nil {
				log.Warn("diagnostics retention startup sweep failed", "error", err)
			}
		}()
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				if err := sessionbundle.RunDiagnosticsRetention(diagnosticsDir, 30*24*time.Hour); err != nil {
					log.Warn("diagnostics retention daily sweep failed", "error", err)
				}
			}
		}()
	}
}

// tokenWiring is the token store, context-history store and the Claude history directory
// they watch; all zero when the history directory cannot be resolved.
type tokenWiring struct {
	store      *tokens.TokenStore
	ctxHistory *contexthistory.Store
	historyDir string
}

// startTokenStores constructs and starts the TokenStore early so QuotaGate's first
// synchronous Reconcile reads real token usage.
func startTokenStores(svc *ServiceDeps, cfg *config.Config, historyLinker *importer.HistoryLinker) tokenWiring {
	storage, eventBus := svc.Storage, svc.EventBus
	// Construct and start TokenStore early — before BacklogController/QuotaGate
	// below — so QuotaGate's very first Reconcile (synchronous, at boot) can
	// read real, restart-durable token-usage data instead of skipping the soft
	// signal entirely for up to 60s. Only tokenStore/historyDir/homeDir are
	// hoisted here; InsightsService construction, backlogSvc.SetTokenStore, and
	// the ArtifactExtractor wiring stay at their original later position
	// (below), now reading these already-constructed outer-scope variables.
	homeDir, homeDirErr := os.UserHomeDir()
	var tokenStore *tokens.TokenStore
	var ctxHistoryStore *contexthistory.Store
	var historyDir string
	if homeDirErr == nil {
		// Under test isolation this resolves inside the isolated config dir
		// rather than the operator's real ~/.claude/projects, which
		// TokenStore.Start / ArtifactExtractor.Start would otherwise walk in
		// full. See config.ResolveClaudeHistoryDir for the full rationale;
		// session.NewHistoryLinkerFromRealInspector's fsnotify watcher
		// resolves the same directory through the same helper.
		historyDir, homeDirErr = config.ResolveClaudeHistoryDir(homeDir, config.IsIsolatedInstance())
	}
	if homeDirErr == nil {
		tokenOpts := []tokens.Option{tokens.WithQueueConfig(scanqueue.BackfillConfig(tokenBackfillDelay))}
		if entClient := storage.GetEntClient(); entClient != nil {
			tokenOpts = append(tokenOpts, tokens.WithStateStore(scanstate.NewStore(entClient).Consumer("tokens")))
		}
		tokenStore = tokens.NewTokenStore(historyDir, tokenOpts...)
		historyLinker.RegisterFileCallback(tokenStore.OnHistoryFileChanged)
		go publishContextHealth(context.Background(), tokenStore,
			historyLinker.Instances,
			func() config.ContextHealthConfig {
				if cfg == nil {
					return config.ContextHealthConfig{}
				}
				return cfg.ContextHealth
			},
			func(inst *instance.Instance) {
				eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"context_health"}))
			})
		tokenStore.Start(context.Background())
		// Persist context history off the parse/CapacityMonitor path: the recorder
		// consumes TokenStore's notifications on its own goroutine.
		if entClient := storage.GetEntClient(); entClient != nil {
			ctxHistoryStore = contexthistory.NewStore(entClient)
			go contexthistory.NewRecorder(ctxHistoryStore, anthropicwire.AnthropicContextWindow, 0).
				Run(context.Background(), tokenStore)
		}
	} else {
		log.Warn("could not resolve Claude history dir for InsightsService token store", "err", homeDirErr)
	}

	return tokenWiring{store: tokenStore, ctxHistory: ctxHistoryStore, historyDir: historyDir}
}

// buildInsights builds InsightsService, the optional Gemini caller and the
// ArtifactExtractor on top of the already-started token store.
func buildInsights(svc *ServiceDeps, tokenStack tokenWiring, historyLinker *importer.HistoryLinker, backlogSvc *backlogsvc.BacklogService,
	llmSelector *headless.Selector, sessionSummaryGenerator *summaries.SessionSummaryGenerator) (*insights.InsightsService, *headless.GeminiCaller) {
	storage, eventBus, sessionService := svc.Storage, svc.EventBus, svc.SessionService
	tokenStore, ctxHistoryStore, historyDir := tokenStack.store, tokenStack.ctxHistory, tokenStack.historyDir
	// Initialize InsightsService for token usage analytics. tokenStore/historyDir
	// were already constructed and started earlier (see the QuotaGate wiring
	// above) so QuotaGate's boot-time Reconcile could read real data — this
	// block only builds the InsightsService/ArtifactExtractor machinery that
	// depends on them, plus backlogSvc/sessionSummaryGenerator, all of which
	// need backlogSvc (constructed after that early wiring).
	var insightsSvc *insights.InsightsService
	var geminiCaller *headless.GeminiCaller
	if tokenStore != nil {
		pricing := tokens.DefaultPricingTable()
		if configDir, cfgErr := config.GetConfigDir(); cfgErr == nil {
			overridePath := filepath.Join(configDir, "pricing_overrides.json")
			if overrideTable, loadErr := tokens.LoadPricingOverride(overridePath); loadErr == nil {
				pricing = overrideTable
			} else if !os.IsNotExist(loadErr) {
				log.Warn("failed to load pricing override, using defaults", "path", overridePath, "err", loadErr)
			}
		} else {
			log.Warn("failed to resolve config dir, skipping pricing override, using defaults", "err", cfgErr)
		}
		if pricing.IsStale() {
			log.Warn("pricing table is stale (an entry's EffectiveDate is 30+ days old)", "loadedAt", pricing.LoadedAt)
		}

		// GeminiCaller (backlog-stage-execution-costs Epic 3.1): only construct it
		// when a gemini binary is actually detected at startup — an absent binary
		// just means this field stays nil, exactly like HeadlessPool above when
		// claude is missing. Detected via a direct exec.LookPath rather than
		// config.GetAvailablePrograms() (a heavier shell-based scan across every
		// candidate CLI) since GeminiCaller.Available() already re-probes with the
		// same exec.LookPath primitive at call time — one detection mechanism for
		// this binary, not two. Nested under tokenStore != nil because it reuses
		// the pricing table already loaded for InsightsService just above, rather
		// than loading pricing a second, independent way.
		if _, lookErr := exec.LookPath("gemini"); lookErr == nil {
			geminiCaller = headless.NewGeminiCaller("gemini", pricing, 5)
			backlogSvc.SetGeminiCaller(geminiCaller)
			llmSelector.Register(headless.NewGeminiBackend(geminiCaller))
			log.Info("gemini headless caller initialized", "maxConcurrent", 5)
		} else {
			log.Warn("gemini headless caller disabled: gemini binary not found", "err", lookErr)
		}

		associator := tokens.NewAssociator(storage)
		insightsSvc = insights.NewInsightsService(tokenStore, pricing, associator, storage)
		insightsSvc.SetDismissedFindingsStore(storage)
		insightsSvc.SetContextHistoryStore(ctxHistoryStore)
		sessionService.SetTokenStoreReader(tokenStore)
		backlogSvc.SetTokenStore(tokenStore, pricing)
		if sessionSummaryGenerator != nil {
			sessionSummaryGenerator.SetTokenStore(tokenStore)
		}
		log.Info("InsightsService initialized", "historyDir", historyDir)

		// Wire ArtifactExtractor to extract PR links, commits, and URLs from JSONL history.
		// Uses historyLinker.Instances() for live instance snapshots so sessions created
		// after startup are included in lookupTitle and OnScanComplete.
		artifactOpts := []artifacts.Option{
			artifacts.WithQueueConfig(scanqueue.BackfillConfig(artifactBackfillDelay)),
			artifacts.WithScanV2Flag(func() bool { return config.LoadConfig().ArtifactScanV2Enabled() }),
		}
		if entClient := storage.GetEntClient(); entClient != nil {
			artifactOpts = append(artifactOpts, artifacts.WithStateStore(scanstate.NewStore(entClient).Consumer("artifacts")))
		}
		artifactExtractor := artifacts.NewArtifactExtractor(
			func(title, blob string) error {
				return storage.UpdateInstanceArtifacts(title, blob)
			},
			func(title string) (string, error) {
				return storage.GetInstanceArtifacts(title)
			},
			func(filePath string) (string, bool) {
				return session.FindInstanceByHistoryPath(historyLinker.Instances(), filePath)
			},
			artifactOpts...,
		)
		artifactIndex := search.NewArtifactIndex(storage)
		attachScan := artifactScanCompleteHandler(historyLinker.Instances, eventBus)
		indexScan := indexScannedArtifacts(historyLinker.Instances, artifactIndex)
		artifactExtractor.OnScanComplete = func(title string, blob *domain.SessionArtifactsBlob) {
			attachScan(title, blob)
			indexScan(title, blob)
		}
		sessionService.SetArtifactIndex(artifactIndex)
		buildArtifactIndexInBackground(historyLinker.Instances, artifactIndex, listenerUp.ch)
		sessionService.SetArtifactRescanner(artifactExtractor)
		historyLinker.RegisterFileCallback(artifactExtractor.OnHistoryFileChanged)
		artifactExtractor.SeedOffsets(session.InstanceInfoSlice(historyLinker.Instances()))
		artifactExtractor.Start(context.Background(), historyDir)
		log.Info("ArtifactExtractor initialized", "historyDir", historyDir)
	}

	return insightsSvc, geminiCaller
}
