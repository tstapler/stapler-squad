# Session Management Web UI Enhancement

## Epic Overview

**Goal**: Transform the basic session management web UI into a fully-featured interface with feature parity to the TUI version, focusing on usability, accessibility, and performance.

**Value Proposition**:
- Enable web-based session management without TUI dependency
- Provide modern, intuitive interface for session lifecycle operations
- Support remote access and team collaboration capabilities
- Reduce learning curve with familiar web UI patterns
- Enable mobile/tablet access for session monitoring

**Success Metrics**:
- Web UI feature parity with TUI (core features implemented)
- <100ms render time for lists with 100+ sessions
- WCAG 2.1 AA accessibility compliance
- Zero regressions in existing functionality
- 90%+ user satisfaction in usability testing
- Responsive design supporting 320px-2560px viewports

**Current State**:
- ✅ Basic session list with cards
- ✅ Status/category/search filtering
- ✅ CRUD operations (create, pause, resume, delete)
- ✅ Real-time updates via ConnectRPC streaming
- ✅ Category-based organization

**Target State**:
- Session detail view with comprehensive information
- Terminal output display and interaction
- Session creation wizard with validation
- Visual diff stats and git branch information
- Enhanced UX with loading states, error handling, keyboard navigation
- Bulk operations and performance monitoring

---

## Story 1: Core UI Foundation & Navigation (3 days)

**Objective**: Establish robust UI foundation with routing, navigation, and layout patterns that support feature expansion.

**Value**: Enables all subsequent features with consistent navigation and state management patterns.

**Dependencies**: None (foundational)

### Task 1.1: Implement React Router and Navigation Structure (2h) - Small

**Scope**: Add React Router for multi-page navigation with session detail views.

**Files**:
- `web-app/package.json` - Add react-router-dom dependency
- `web-app/src/app/layout.tsx` - Integrate router provider
- `web-app/src/app/page.tsx` - Update to use router navigation
- `web-app/src/lib/routes.ts` - New file for route definitions

**Context Needed**:
- Next.js 15 app router patterns
- React Router integration with Next.js
- Current page structure

**Implementation**:
```typescript
// web-app/src/lib/routes.ts
export const routes = {
  home: "/",
  sessionDetail: (id: string) => `/sessions/${id}`,
  sessionCreate: "/sessions/new",
  settings: "/settings",
} as const;

// web-app/src/app/layout.tsx
import { BrowserRouter } from "react-router-dom";

export default function RootLayout({ children }) {
  return (
    <html>
      <body>
        <BrowserRouter>
          <Navigation />
          {children}
        </BrowserRouter>
      </body>
    </html>
  );
}
```

**Success Criteria**:
- Navigation between pages without full reload
- Browser back/forward buttons work correctly
- URL reflects current view
- Type-safe route helpers available

**Testing**:
```bash
npm run dev
# Navigate to http://localhost:3001
# Click session cards and verify URL changes
# Use browser back button
# Verify no console errors
```

**Dependencies**: None

---

### Task 1.2: Create Loading States and Skeletons (2h) - Small

**Scope**: Implement skeleton loaders and loading states for all async operations.

**Files**:
- `web-app/src/components/ui/Skeleton.tsx` - New skeleton component
- `web-app/src/components/sessions/SessionCardSkeleton.tsx` - New skeleton for cards
- `web-app/src/components/sessions/SessionList.tsx` - Integrate loading states
- `web-app/src/app/page.tsx` - Update loading display

**Context Needed**:
- Current loading patterns in useSessionService
- React Suspense patterns
- Skeleton UI best practices

**Implementation**:
```typescript
// web-app/src/components/ui/Skeleton.tsx
export function Skeleton({ className, ...props }: SkeletonProps) {
  return (
    <div
      className={cn("animate-pulse rounded-md bg-gray-200", className)}
      {...props}
    />
  );
}

// web-app/src/components/sessions/SessionCardSkeleton.tsx
export function SessionCardSkeleton() {
  return (
    <div className={styles.card}>
      <Skeleton className="h-6 w-3/4 mb-2" />
      <Skeleton className="h-4 w-1/2 mb-4" />
      <Skeleton className="h-20 w-full" />
    </div>
  );
}
```

**Success Criteria**:
- Smooth loading transitions without content jumps
- Skeleton matches actual content layout
- Loading states for all async operations
- Accessible loading announcements

**Testing**:
- Throttle network in DevTools to 3G
- Verify skeleton displays during load
- Check screen reader announces loading state

**Dependencies**: Task 1.1

---

### Task 1.3: Implement Error Boundary and Error States (2h) - Small

**Scope**: Add comprehensive error handling with retry mechanisms and user feedback.

**Files**:
- `web-app/src/components/ui/ErrorBoundary.tsx` - New error boundary component
- `web-app/src/components/ui/ErrorState.tsx` - New error display component
- `web-app/src/app/layout.tsx` - Wrap with error boundary
- `web-app/src/lib/hooks/useSessionService.ts` - Add retry logic

**Context Needed**:
- React error boundaries
- ConnectRPC error handling
- Current error propagation patterns

**Implementation**:
```typescript
// web-app/src/components/ui/ErrorBoundary.tsx
export class ErrorBoundary extends React.Component<Props, State> {
  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("Error caught by boundary:", error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return <ErrorState error={this.state.error} onRetry={this.props.onRetry} />;
    }
    return this.props.children;
  }
}

// web-app/src/lib/hooks/useSessionService.ts
const retryOperation = async (operation: () => Promise<T>, maxRetries = 3) => {
  for (let i = 0; i < maxRetries; i++) {
    try {
      return await operation();
    } catch (err) {
      if (i === maxRetries - 1) throw err;
      await new Promise(resolve => setTimeout(resolve, 1000 * (i + 1)));
    }
  }
};
```

**Success Criteria**:
- Graceful error display with actionable messages
- Retry mechanism works for transient failures
- Error boundary catches component errors
- Errors logged for debugging

**Testing**:
- Disconnect network and verify error display
- Click retry button and verify recovery
- Throw error in component and verify boundary catches
- Check console for error logs

**Dependencies**: Task 1.1

---

### Task 1.4: Add Keyboard Navigation Support (3h) - Medium

**Scope**: Implement comprehensive keyboard navigation for power users and accessibility.

**Files**:
- `web-app/src/lib/hooks/useKeyboard.ts` - New keyboard navigation hook
- `web-app/src/components/sessions/SessionList.tsx` - Add keyboard handlers
- `web-app/src/components/sessions/SessionCard.tsx` - Make focusable and navigable
- `web-app/src/app/page.tsx` - Integrate keyboard shortcuts

**Context Needed**:
- Web accessibility keyboard patterns (arrow keys, Enter, Space)
- Focus management in React
- Keyboard shortcut best practices

**Implementation**:
```typescript
// web-app/src/lib/hooks/useKeyboard.ts
export function useKeyboard(handlers: KeyboardHandlers) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (handlers[e.key]) {
        e.preventDefault();
        handlers[e.key]();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handlers]);
}

// web-app/src/components/sessions/SessionList.tsx
useKeyboard({
  ArrowDown: () => setSelectedIndex(i => Math.min(i + 1, sessions.length - 1)),
  ArrowUp: () => setSelectedIndex(i => Math.max(i - 1, 0)),
  Enter: () => sessions[selectedIndex] && onSessionClick(sessions[selectedIndex]),
  "/": () => setSearchFocused(true),
});
```

**Success Criteria**:
- Arrow keys navigate between sessions
- Enter opens selected session
- "/" focuses search input
- Tab order logical and complete
- Keyboard shortcuts discoverable (help modal)

**Testing**:
- Navigate entire UI using only keyboard
- Verify focus indicators visible
- Test with screen reader
- Check shortcut conflicts with browser

**Dependencies**: Task 1.1

---

## Story 2: Session Detail View (4 days)

**Objective**: Implement comprehensive session detail view with terminal output, diff visualization, and full session information.

**Value**: Enables users to view and interact with individual sessions in depth, matching TUI capabilities.

**Dependencies**: Story 1 (navigation structure)

### Task 2.1: Create Session Detail Layout and Routing (2h) - Small

**Scope**: Set up detail page layout with tabbed interface for different views.

**Files**:
- `web-app/src/app/sessions/[id]/page.tsx` - New detail page
- `web-app/src/components/sessions/SessionDetail.tsx` - New detail container
- `web-app/src/components/ui/Tabs.tsx` - New tabs component
- `web-app/src/lib/hooks/useSession.ts` - New hook for single session

**Context Needed**:
- Next.js dynamic routes
- Session data structure from protobuf
- Current session card layout

**Implementation**:
```typescript
// web-app/src/app/sessions/[id]/page.tsx
export default function SessionDetailPage({ params }: { params: { id: string } }) {
  const { session, loading, error } = useSession(params.id);

  if (loading) return <SessionDetailSkeleton />;
  if (error) return <ErrorState error={error} />;
  if (!session) return <NotFound />;

  return <SessionDetail session={session} />;
}

// web-app/src/components/sessions/SessionDetail.tsx
export function SessionDetail({ session }: Props) {
  return (
    <div className={styles.container}>
      <SessionHeader session={session} />
      <Tabs defaultValue="terminal">
        <TabsList>
          <Tab value="terminal">Terminal</Tab>
          <Tab value="diff">Diff</Tab>
          <Tab value="logs">Logs</Tab>
          <Tab value="info">Info</Tab>
        </TabsList>
        <TabsContent value="terminal"><TerminalView session={session} /></TabsContent>
        <TabsContent value="diff"><DiffView session={session} /></TabsContent>
        <TabsContent value="logs"><LogsView session={session} /></TabsContent>
        <TabsContent value="info"><InfoView session={session} /></TabsContent>
      </Tabs>
    </div>
  );
}
```

**Success Criteria**:
- Detail page accessible via URL with session ID
- Tabs switch without reload
- Back button returns to list
- Active tab persisted in URL

**Testing**:
- Navigate to /sessions/{id}
- Switch tabs and verify URL updates
- Reload page and verify active tab restored
- Share URL and verify recipient sees same view

**Dependencies**: Task 1.1

---

### Task 2.2: Implement Terminal Output Display (3h) - Medium

**Scope**: Create terminal output viewer with ANSI color support and auto-scroll.

**Files**:
- `web-app/src/components/terminal/TerminalView.tsx` - New terminal viewer
- `web-app/src/lib/hooks/useTerminalStream.ts` - New terminal streaming hook
- `web-app/package.json` - Add xterm.js dependency
- `web-app/src/components/terminal/TerminalView.module.css` - Terminal styles

**Context Needed**:
- xterm.js library API
- ConnectRPC streaming pattern from useSessionService
- Terminal data structure from protobuf
- PTY streaming implementation in server

**Implementation**:
```typescript
// web-app/src/components/terminal/TerminalView.tsx
import { Terminal } from "xterm";
import "xterm/css/xterm.css";

export function TerminalView({ session }: Props) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const { output, connected } = useTerminalStream(session.id);

  useEffect(() => {
    if (!terminalRef.current) return;

    const term = new Terminal({
      theme: { background: "#1e1e1e" },
      fontSize: 14,
      fontFamily: "Menlo, Monaco, 'Courier New', monospace",
    });

    term.open(terminalRef.current);

    return () => term.dispose();
  }, []);

  useEffect(() => {
    if (output) {
      terminal.write(output);
    }
  }, [output]);

  return (
    <div className={styles.terminal}>
      <div ref={terminalRef} className={styles.terminalContent} />
      <div className={styles.status}>
        {connected ? "Connected" : "Disconnected"}
      </div>
    </div>
  );
}
```

**Success Criteria**:
- Terminal displays session output with correct formatting
- ANSI colors rendered correctly
- Auto-scroll to bottom on new output
- Manual scroll prevents auto-scroll
- Connection status displayed

**Testing**:
- Open session with active output
- Verify colors and formatting match tmux
- Scroll up and verify auto-scroll pauses
- Scroll to bottom and verify auto-scroll resumes
- Check performance with high-frequency updates

**Dependencies**: Task 2.1

---

### Task 2.3: Add Terminal Input and Bidirectional Communication (3h) - Medium

**Scope**: Enable terminal input with bidirectional streaming to send commands to sessions.

**Files**:
- `web-app/src/components/terminal/TerminalView.tsx` - Add input handling
- `web-app/src/lib/hooks/useTerminalStream.ts` - Add input streaming
- `server/session_service.go` - Verify StreamTerminal implementation
- `web-app/src/components/terminal/TerminalControls.tsx` - New control bar

**Context Needed**:
- xterm.js input handling (onData event)
- ConnectRPC bidirectional streaming
- Current StreamTerminal RPC implementation
- Terminal control sequences

**Implementation**:
```typescript
// web-app/src/lib/hooks/useTerminalStream.ts
export function useTerminalStream(sessionId: string) {
  const [output, setOutput] = useState("");
  const [connected, setConnected] = useState(false);
  const streamRef = useRef<AsyncIterableIterator<TerminalData>>();

  const sendInput = useCallback(async (data: string) => {
    if (!clientRef.current) return;

    try {
      // Send terminal input via bidirectional stream
      await clientRef.current.streamTerminal((async function* () {
        yield new TerminalData({
          sessionId,
          data: new TextEncoder().encode(data),
        });
      })());
    } catch (err) {
      console.error("Failed to send input:", err);
    }
  }, [sessionId]);

  return { output, connected, sendInput };
}

// web-app/src/components/terminal/TerminalView.tsx
term.onData((data) => {
  sendInput(data);
});
```

**Success Criteria**:
- Typing in terminal sends to session
- Commands execute correctly in remote tmux
- Special keys (arrows, Ctrl+C) work correctly
- Input echoing handled correctly

**Testing**:
- Type commands and verify execution
- Test arrow keys for history navigation
- Test Ctrl+C interrupt
- Test tab completion
- Verify no input duplication

**Dependencies**: Task 2.2

---

### Task 2.4: Create Diff Visualization Component (3h) - Medium

**Scope**: Build visual diff display with syntax highlighting for git changes.

**Files**:
- `web-app/src/components/diff/DiffView.tsx` - New diff viewer
- `web-app/src/lib/hooks/useSessionDiff.ts` - New diff fetching hook
- `web-app/package.json` - Add react-diff-viewer-continued dependency
- `web-app/src/components/diff/DiffView.module.css` - Diff styles

**Context Needed**:
- GetSessionDiff RPC from protobuf
- Git diff format and structure
- react-diff-viewer library API
- Current DiffStats display in SessionCard

**Implementation**:
```typescript
// web-app/src/lib/hooks/useSessionDiff.ts
export function useSessionDiff(sessionId: string) {
  const [diff, setDiff] = useState<DiffStats | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const fetchDiff = async () => {
      setLoading(true);
      try {
        const response = await client.getSessionDiff({ id: sessionId });
        setDiff(response.diffStats);
      } catch (err) {
        console.error("Failed to fetch diff:", err);
      } finally {
        setLoading(false);
      }
    };

    fetchDiff();
    const interval = setInterval(fetchDiff, 5000); // Refresh every 5s
    return () => clearInterval(interval);
  }, [sessionId]);

  return { diff, loading };
}

// web-app/src/components/diff/DiffView.tsx
import ReactDiffViewer from "react-diff-viewer-continued";

export function DiffView({ session }: Props) {
  const { diff, loading } = useSessionDiff(session.id);

  if (loading) return <DiffSkeleton />;
  if (!diff?.content) return <EmptyDiff />;

  return (
    <div className={styles.diffContainer}>
      <div className={styles.stats}>
        <span className={styles.added}>+{diff.added}</span>
        <span className={styles.removed}>-{diff.removed}</span>
      </div>
      <ReactDiffViewer
        oldValue=""
        newValue={diff.content}
        splitView={false}
        useDarkTheme={true}
        leftTitle="Base"
        rightTitle="Changes"
      />
    </div>
  );
}
```

**Success Criteria**:
- Diff displays with syntax highlighting
- Added/removed lines clearly distinguished
- Diff stats summary visible
- Real-time updates every 5 seconds
- Large diffs render performantly

**Testing**:
- Make changes in session
- Verify diff updates within 5 seconds
- Check added/removed line colors
- Test with large diffs (1000+ lines)
- Verify syntax highlighting for different file types

**Dependencies**: Task 2.1

---

### Task 2.5: Build Session Info Tab (2h) - Small

**Scope**: Display comprehensive session metadata and configuration.

**Files**:
- `web-app/src/components/sessions/InfoView.tsx` - New info display
- `web-app/src/components/sessions/SessionTimeline.tsx` - New timeline component
- `web-app/src/components/sessions/InfoView.module.css` - Info styles

**Context Needed**:
- Complete Session data structure from protobuf
- Current SessionCard info display
- Timeline UI patterns

**Implementation**:
```typescript
// web-app/src/components/sessions/InfoView.tsx
export function InfoView({ session }: Props) {
  return (
    <div className={styles.infoContainer}>
      <Section title="General">
        <InfoRow label="ID" value={session.id} />
        <InfoRow label="Status" value={getStatusText(session.status)} />
        <InfoRow label="Program" value={session.program} />
        <InfoRow label="Category" value={session.category || "Uncategorized"} />
      </Section>

      <Section title="Git">
        <InfoRow label="Branch" value={session.branch} />
        <InfoRow label="Repository" value={session.path} />
        <InfoRow label="Working Directory" value={session.workingDir} />
      </Section>

      <Section title="Timeline">
        <SessionTimeline
          created={session.createdAt}
          updated={session.updatedAt}
          events={session.events}
        />
      </Section>

      <Section title="Resources">
        <InfoRow label="tmux Session" value={`claudesquad_${session.title}`} />
        <InfoRow label="Worktree Path" value={session.worktreePath} />
      </Section>
    </div>
  );
}
```

**Success Criteria**:
- All session metadata displayed clearly
- Timestamps formatted correctly
- Copyable values (session ID, paths)
- Responsive layout

**Testing**:
- View info for various session types
- Copy values and verify clipboard
- Check mobile layout
- Verify all fields populated

**Dependencies**: Task 2.1

---

## Story 3: Session Creation Wizard (3 days)

**Objective**: Build intuitive session creation flow with validation, contextual discovery, and preset templates.

**Value**: Simplifies session creation with guided workflow and reduces errors through validation.

**Dependencies**: Story 1 (navigation and error handling)

### Task 3.1: Create Multi-Step Session Creation Form (3h) - Medium

**Scope**: Build wizard-style form with validation for session creation.

**Files**:
- `web-app/src/app/sessions/new/page.tsx` - New creation page
- `web-app/src/components/sessions/SessionWizard.tsx` - New wizard component
- `web-app/src/components/ui/Wizard.tsx` - Reusable wizard component
- `web-app/src/lib/validation/sessionSchema.ts` - New validation schema

**Context Needed**:
- CreateSessionRequest structure from protobuf
- Form validation patterns (zod or yup)
- Multi-step form patterns in React
- Current session creation in TUI

**Implementation**:
```typescript
// web-app/src/lib/validation/sessionSchema.ts
import { z } from "zod";

export const sessionSchema = z.object({
  title: z.string().min(1, "Title required").max(100),
  path: z.string().min(1, "Path required"),
  workingDir: z.string().optional(),
  branch: z.string().optional(),
  program: z.string().default("claude"),
  category: z.string().optional(),
  prompt: z.string().optional(),
  autoYes: z.boolean().default(false),
  existingWorktree: z.string().optional(),
});

// web-app/src/components/sessions/SessionWizard.tsx
export function SessionWizard({ onComplete }: Props) {
  const [step, setStep] = useState(1);
  const { register, handleSubmit, formState: { errors } } = useForm({
    resolver: zodResolver(sessionSchema),
  });

  const steps = [
    { title: "Basic Info", fields: ["title", "category"] },
    { title: "Repository", fields: ["path", "workingDir", "branch"] },
    { title: "Configuration", fields: ["program", "prompt", "autoYes"] },
  ];

  return (
    <Wizard currentStep={step} steps={steps.map(s => s.title)}>
      <form onSubmit={handleSubmit(onComplete)}>
        {step === 1 && <BasicInfoStep register={register} errors={errors} />}
        {step === 2 && <RepositoryStep register={register} errors={errors} />}
        {step === 3 && <ConfigStep register={register} errors={errors} />}

        <WizardActions>
          <Button onClick={() => setStep(s => s - 1)} disabled={step === 1}>
            Back
          </Button>
          {step < 3 ? (
            <Button onClick={() => setStep(s => s + 1)}>Next</Button>
          ) : (
            <Button type="submit">Create Session</Button>
          )}
        </WizardActions>
      </form>
    </Wizard>
  );
}
```

**Success Criteria**:
- Three-step wizard with clear progression
- Field validation on each step
- Cannot proceed with invalid data
- Form state persisted across steps
- Success/error feedback on submission

**Testing**:
- Complete wizard with valid data
- Try to proceed with invalid data
- Go back and verify field values preserved
- Submit and verify session created
- Test all validation rules

**Dependencies**: Task 1.3 (error handling)

---

### Task 3.2: Add Path Discovery and Auto-Fill (2h) - Small

**Scope**: Implement contextual path discovery with git repository detection.

**Files**:
- `web-app/src/components/sessions/PathInput.tsx` - Enhanced path input
- `web-app/src/lib/hooks/usePathDiscovery.ts` - New discovery hook
- `web-app/src/components/sessions/SessionWizard.tsx` - Integrate discovery

**Context Needed**:
- TUI path discovery logic in `ui/overlay/sessionSetup.go`
- Git repository detection patterns
- File system path validation

**Implementation**:
```typescript
// web-app/src/lib/hooks/usePathDiscovery.ts
export function usePathDiscovery(path: string) {
  const [suggestions, setSuggestions] = useState<PathSuggestion[]>([]);
  const [isGitRepo, setIsGitRepo] = useState(false);

  useEffect(() => {
    const discoverPath = async () => {
      if (!path) return;

      try {
        // Check if path is a git repository
        const response = await fetch(`/api/validate-path?path=${encodeURIComponent(path)}`);
        const data = await response.json();

        setIsGitRepo(data.isGitRepo);
        setSuggestions(data.suggestions || []);
      } catch (err) {
        console.error("Path discovery failed:", err);
      }
    };

    const debounce = setTimeout(discoverPath, 300);
    return () => clearTimeout(debounce);
  }, [path]);

  return { suggestions, isGitRepo };
}

// web-app/src/components/sessions/PathInput.tsx
export function PathInput({ value, onChange, onSelect }: Props) {
  const { suggestions, isGitRepo } = usePathDiscovery(value);

  return (
    <div className={styles.pathInput}>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="/path/to/repository"
      />
      {isGitRepo && <Badge variant="success">Git Repository</Badge>}
      {suggestions.length > 0 && (
        <SuggestionList>
          {suggestions.map((s) => (
            <SuggestionItem key={s.path} onClick={() => onSelect(s)}>
              {s.path} {s.branch && `(${s.branch})`}
            </SuggestionItem>
          ))}
        </SuggestionList>
      )}
    </div>
  );
}
```

**Success Criteria**:
- Typing path shows suggestions
- Git repositories detected automatically
- Selecting suggestion fills all relevant fields
- Invalid paths show clear feedback
- Debounced validation (300ms)

**Testing**:
- Type git repository path
- Verify "Git Repository" badge appears
- Type non-git path
- Verify no badge
- Select suggestion and verify form filled

**Dependencies**: Task 3.1

---

### Task 3.3: Implement Session Templates (2h) - Small

**Scope**: Add preset templates for common session configurations.

**Files**:
- `web-app/src/lib/templates/sessionTemplates.ts` - New template definitions
- `web-app/src/components/sessions/TemplateSelector.tsx` - New template picker
- `web-app/src/components/sessions/SessionWizard.tsx` - Integrate templates

**Context Needed**:
- Common session configurations from TUI usage
- Template pattern for pre-filled forms
- Session schema from validation

**Implementation**:
```typescript
// web-app/src/lib/templates/sessionTemplates.ts
export const templates: SessionTemplate[] = [
  {
    id: "claude-default",
    name: "Claude Code (Default)",
    description: "Standard Claude Code session with git worktree",
    config: {
      program: "claude",
      autoYes: false,
    },
  },
  {
    id: "aider-ollama",
    name: "Aider with Ollama",
    description: "Local Aider session using Ollama models",
    config: {
      program: "aider --model ollama_chat/gemma3:1b",
      autoYes: false,
    },
  },
  {
    id: "quick-experiment",
    name: "Quick Experiment",
    description: "Auto-approve session for rapid testing",
    config: {
      program: "claude",
      autoYes: true,
      category: "Experiments",
    },
  },
];

// web-app/src/components/sessions/TemplateSelector.tsx
export function TemplateSelector({ onSelect }: Props) {
  return (
    <div className={styles.templates}>
      <h3>Start from a template</h3>
      <div className={styles.grid}>
        {templates.map((template) => (
          <TemplateCard
            key={template.id}
            template={template}
            onClick={() => onSelect(template)}
          />
        ))}
      </div>
      <Button variant="ghost" onClick={() => onSelect(null)}>
        Start blank
      </Button>
    </div>
  );
}
```

**Success Criteria**:
- Template selector displayed before wizard
- Selecting template pre-fills form
- "Start blank" option available
- Templates saved in localStorage
- Custom templates can be saved

**Testing**:
- Select each template
- Verify form pre-filled correctly
- Create session from template
- Save custom template
- Reload and verify templates persisted

**Dependencies**: Task 3.1

---

## Story 4: Bulk Operations & Advanced Features (2 days)

**Objective**: Enable power-user features like bulk operations, advanced filtering, and performance monitoring.

**Value**: Improves efficiency for users managing many sessions simultaneously.

**Dependencies**: Story 1, Story 2

### Task 4.1: Add Multi-Select and Bulk Actions (3h) - Medium

**Scope**: Implement checkbox-based multi-select with bulk pause/resume/delete.

**Files**:
- `web-app/src/components/sessions/SessionList.tsx` - Add multi-select mode
- `web-app/src/components/sessions/BulkActions.tsx` - New bulk action bar
- `web-app/src/lib/hooks/useSelection.ts` - New selection management hook
- `web-app/src/lib/hooks/useSessionService.ts` - Add bulk operation methods

**Context Needed**:
- Multi-select UI patterns
- Current CRUD operations in useSessionService
- Optimistic updates for bulk operations

**Implementation**:
```typescript
// web-app/src/lib/hooks/useSelection.ts
export function useSelection<T extends { id: string }>(items: T[]) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [selectMode, setSelectMode] = useState(false);

  const toggle = (id: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const selectAll = () => setSelected(new Set(items.map(i => i.id)));
  const clearSelection = () => setSelected(new Set());

  return { selected, selectMode, setSelectMode, toggle, selectAll, clearSelection };
}

// web-app/src/components/sessions/BulkActions.tsx
export function BulkActions({ selectedIds, onPause, onResume, onDelete }: Props) {
  return (
    <div className={styles.bulkActions}>
      <span>{selectedIds.length} selected</span>
      <Button onClick={() => onPause(selectedIds)}>Pause All</Button>
      <Button onClick={() => onResume(selectedIds)}>Resume All</Button>
      <Button variant="danger" onClick={() => onDelete(selectedIds)}>
        Delete All
      </Button>
    </div>
  );
}
```

**Success Criteria**:
- Checkbox appears on each session card
- Bulk action bar appears when items selected
- Bulk operations execute in parallel
- Progress indicator during bulk operations
- Undo capability for bulk delete

**Testing**:
- Select multiple sessions
- Execute bulk pause
- Verify all sessions paused
- Test bulk delete with confirmation
- Check performance with 50+ selections

**Dependencies**: Task 1.1, Task 1.3

---

### Task 4.2: Implement Advanced Filtering (2h) - Small

**Scope**: Add advanced filter modal with multiple criteria and saved filters.

**Files**:
- `web-app/src/components/sessions/FilterModal.tsx` - New advanced filter modal
- `web-app/src/components/sessions/SessionList.tsx` - Integrate advanced filters
- `web-app/src/lib/hooks/useFilters.ts` - New filter management hook

**Context Needed**:
- Current filtering in SessionList
- Filter combination logic (AND/OR)
- localStorage for saved filters

**Implementation**:
```typescript
// web-app/src/lib/hooks/useFilters.ts
export function useFilters() {
  const [filters, setFilters] = useState<FilterConfig>({
    status: [],
    category: [],
    searchQuery: "",
    dateRange: null,
  });

  const applyFilters = (sessions: Session[]) => {
    return sessions.filter(session => {
      if (filters.status.length > 0 && !filters.status.includes(session.status)) {
        return false;
      }
      if (filters.category.length > 0 && !filters.category.includes(session.category)) {
        return false;
      }
      if (filters.searchQuery) {
        const query = filters.searchQuery.toLowerCase();
        if (!session.title.toLowerCase().includes(query) &&
            !session.path.toLowerCase().includes(query)) {
          return false;
        }
      }
      return true;
    });
  };

  return { filters, setFilters, applyFilters };
}
```

**Success Criteria**:
- Advanced filter modal accessible
- Multiple filter criteria combinable
- Filters saved to localStorage
- Quick filter presets available
- Filter count badge on button

**Testing**:
- Open advanced filter modal
- Apply multiple filters
- Save filter preset
- Reload and verify preset available
- Clear all filters

**Dependencies**: Task 1.1

---

### Task 4.3: Build Performance Dashboard (3h) - Medium

**Scope**: Create dashboard showing session statistics and resource usage.

**Files**:
- `web-app/src/app/dashboard/page.tsx` - New dashboard page
- `web-app/src/components/dashboard/SessionStats.tsx` - Stats visualization
- `web-app/src/components/dashboard/ResourceChart.tsx` - Resource usage chart
- `web-app/src/lib/hooks/useSessionStats.ts` - Stats calculation hook

**Context Needed**:
- Session lifecycle states
- Resource usage patterns
- Chart library (recharts or similar)

**Implementation**:
```typescript
// web-app/src/lib/hooks/useSessionStats.ts
export function useSessionStats(sessions: Session[]) {
  return useMemo(() => {
    const stats = {
      total: sessions.length,
      byStatus: {} as Record<SessionStatus, number>,
      byCategory: {} as Record<string, number>,
      avgAge: 0,
      activeToday: 0,
    };

    sessions.forEach(session => {
      // Count by status
      stats.byStatus[session.status] = (stats.byStatus[session.status] || 0) + 1;

      // Count by category
      const cat = session.category || "Uncategorized";
      stats.byCategory[cat] = (stats.byCategory[cat] || 0) + 1;

      // Calculate average age
      const age = Date.now() - Number(session.createdAt.seconds) * 1000;
      stats.avgAge += age / sessions.length;

      // Count active today
      const updated = Number(session.updatedAt.seconds) * 1000;
      if (Date.now() - updated < 24 * 60 * 60 * 1000) {
        stats.activeToday++;
      }
    });

    return stats;
  }, [sessions]);
}

// web-app/src/components/dashboard/SessionStats.tsx
export function SessionStats({ stats }: Props) {
  return (
    <div className={styles.stats}>
      <StatCard title="Total Sessions" value={stats.total} />
      <StatCard title="Active Today" value={stats.activeToday} />
      <StatCard title="Avg Age" value={formatDuration(stats.avgAge)} />

      <ChartCard title="By Status">
        <PieChart data={Object.entries(stats.byStatus)} />
      </ChartCard>

      <ChartCard title="By Category">
        <BarChart data={Object.entries(stats.byCategory)} />
      </ChartCard>
    </div>
  );
}
```

**Success Criteria**:
- Dashboard shows key metrics
- Charts update in real-time
- Responsive layout
- Export data as CSV
- Link to filtered views from stats

**Testing**:
- View dashboard with various session counts
- Verify charts update with session changes
- Test responsive layout
- Export CSV and verify data
- Click chart segments to filter list

**Dependencies**: Task 1.1

---

## Story 5: Mobile & Accessibility (2 days)

**Objective**: Ensure mobile responsiveness and WCAG 2.1 AA accessibility compliance.

**Value**: Enables mobile monitoring and makes application accessible to all users.

**Dependencies**: All previous stories

### Task 5.1: Implement Responsive Mobile Layout (3h) - Medium

**Scope**: Adapt all components for mobile viewports with touch-friendly interactions.

**Files**:
- `web-app/src/app/globals.css` - Add responsive breakpoints
- `web-app/src/components/sessions/SessionCard.module.css` - Mobile styles
- `web-app/src/components/sessions/SessionList.module.css` - Mobile list
- `web-app/src/components/terminal/TerminalView.module.css` - Mobile terminal

**Context Needed**:
- Mobile UI patterns
- Touch gesture handling
- Responsive design breakpoints
- Current desktop layout

**Implementation**:
```css
/* web-app/src/app/globals.css */
:root {
  --breakpoint-sm: 640px;
  --breakpoint-md: 768px;
  --breakpoint-lg: 1024px;
  --breakpoint-xl: 1280px;
}

/* web-app/src/components/sessions/SessionCard.module.css */
.card {
  padding: 1rem;
}

@media (max-width: 768px) {
  .card {
    padding: 0.75rem;
  }

  .infoRow {
    flex-direction: column;
    align-items: flex-start;
  }

  .actions {
    flex-direction: column;
    gap: 0.5rem;
  }

  .actionButton {
    width: 100%;
  }
}

@media (max-width: 640px) {
  .card {
    padding: 0.5rem;
  }

  .title {
    font-size: 1rem;
  }
}
```

**Success Criteria**:
- All pages usable on 320px-2560px viewports
- Touch targets minimum 44x44px
- No horizontal scroll on mobile
- Optimized layouts for tablet
- Swipe gestures for navigation

**Testing**:
- Test on iPhone SE (375px)
- Test on iPad (768px)
- Test on desktop (1920px)
- Use Chrome DevTools device emulation
- Test actual mobile devices

**Dependencies**: All previous tasks

---

### Task 5.2: Ensure WCAG 2.1 AA Compliance (3h) - Medium

**Scope**: Audit and fix accessibility issues for keyboard, screen reader, and visual compliance.

**Files**:
- `web-app/src/components/` - All component files
- `web-app/src/app/globals.css` - Accessibility styles
- `web-app/.eslintrc.json` - Add jsx-a11y plugin

**Context Needed**:
- WCAG 2.1 AA requirements
- ARIA attributes and roles
- Keyboard navigation patterns
- Color contrast requirements (4.5:1)

**Implementation**:
```typescript
// Add ARIA attributes to all interactive components
<button
  aria-label="Pause session"
  aria-pressed={isPaused}
  onClick={onPause}
>
  <PauseIcon aria-hidden="true" />
  {isPaused ? "Resume" : "Pause"}
</button>

// Add live regions for dynamic updates
<div
  role="status"
  aria-live="polite"
  aria-atomic="true"
  className="sr-only"
>
  {sessions.length} sessions loaded
</div>

// Ensure proper heading hierarchy
<h1>Stapler Squad Sessions</h1>
<section>
  <h2>Active Sessions</h2>
  <article>
    <h3>{session.title}</h3>
  </article>
</section>
```

**Success Criteria**:
- All interactive elements keyboard accessible
- Screen reader announces all UI changes
- Color contrast ratios meet 4.5:1
- No automated accessibility violations (axe-core)
- Focus indicators visible on all elements

**Testing**:
- Run axe DevTools extension
- Test with NVDA/JAWS screen readers
- Navigate entire UI with keyboard only
- Check color contrast with tool
- Validate with Lighthouse accessibility audit

**Dependencies**: All previous tasks

---

### Task 5.3: Add Touch Gestures and Mobile Optimizations (2h) - Small

**Scope**: Implement swipe gestures and mobile-specific optimizations.

**Files**:
- `web-app/src/lib/hooks/useSwipe.ts` - New swipe gesture hook
- `web-app/src/components/sessions/SessionCard.tsx` - Add swipe actions
- `web-app/src/components/sessions/SessionList.tsx` - Add pull-to-refresh

**Context Needed**:
- Touch event handling in React
- Gesture libraries (hammer.js or custom)
- Mobile UX patterns

**Implementation**:
```typescript
// web-app/src/lib/hooks/useSwipe.ts
export function useSwipe(onSwipeLeft?: () => void, onSwipeRight?: () => void) {
  const touchStart = useRef<{ x: number; y: number } | null>(null);

  const handleTouchStart = (e: TouchEvent) => {
    touchStart.current = {
      x: e.touches[0].clientX,
      y: e.touches[0].clientY,
    };
  };

  const handleTouchEnd = (e: TouchEvent) => {
    if (!touchStart.current) return;

    const deltaX = e.changedTouches[0].clientX - touchStart.current.x;
    const deltaY = Math.abs(e.changedTouches[0].clientY - touchStart.current.y);

    // Horizontal swipe (deltaX > threshold and deltaY < threshold)
    if (Math.abs(deltaX) > 50 && deltaY < 30) {
      if (deltaX > 0) {
        onSwipeRight?.();
      } else {
        onSwipeLeft?.();
      }
    }

    touchStart.current = null;
  };

  return { handleTouchStart, handleTouchEnd };
}

// web-app/src/components/sessions/SessionCard.tsx
const { handleTouchStart, handleTouchEnd } = useSwipe(
  () => onDelete?.(), // Swipe left to delete
  () => onPause?.()   // Swipe right to pause
);

<div
  onTouchStart={handleTouchStart}
  onTouchEnd={handleTouchEnd}
  className={styles.card}
>
  {/* card content */}
</div>
```

**Success Criteria**:
- Swipe left to delete session
- Swipe right to pause/resume
- Pull down to refresh session list
- Haptic feedback on actions (if supported)
- Visual feedback during gestures

**Testing**:
- Test swipe gestures on mobile device
- Verify swipe threshold appropriate
- Test pull-to-refresh
- Check accidental gesture prevention
- Verify haptic feedback works

**Dependencies**: Task 5.1

---

## Dependency Visualization

```
Story 1 (Foundation - 3 days)
├─ Task 1.1: React Router [START] ─────────┐
├─ Task 1.2: Loading States         <──────┤
├─ Task 1.3: Error Handling         <──────┤
└─ Task 1.4: Keyboard Navigation    <──────┘

Story 2 (Detail View - 4 days)           <─ (requires Story 1)
├─ Task 2.1: Detail Layout [START]
├─ Task 2.2: Terminal Display       <──────┤
├─ Task 2.3: Terminal Input         <──────┤ (requires 2.2)
├─ Task 2.4: Diff Visualization     <──────┤ (parallel with 2.2-2.3)
└─ Task 2.5: Info Tab              <──────┘

Story 3 (Creation Wizard - 3 days)       <─ (requires Story 1)
├─ Task 3.1: Wizard Form [START]
├─ Task 3.2: Path Discovery         <──────┤
└─ Task 3.3: Templates              <──────┘

Story 4 (Bulk Ops - 2 days)              <─ (requires Story 1, 2)
├─ Task 4.1: Multi-Select [START]
├─ Task 4.2: Advanced Filtering     <──────┤ (parallel)
└─ Task 4.3: Performance Dashboard  <──────┘ (parallel)

Story 5 (Mobile & A11y - 2 days)         <─ (requires all previous)
├─ Task 5.1: Responsive Layout [START]
├─ Task 5.2: Accessibility          <──────┤
└─ Task 5.3: Touch Gestures         <──────┘ (requires 5.1)
```

### Parallel Execution Opportunities

**Phase 1** (After Story 1 complete):
- Story 2 (Detail View)
- Story 3 (Creation Wizard)
Both can be developed in parallel by different team members.

**Phase 2** (After Story 2 and 3):
- Story 4 tasks can all run in parallel:
  - Task 4.1: Multi-Select
  - Task 4.2: Advanced Filtering
  - Task 4.3: Performance Dashboard

**Phase 3** (Sequential):
- Story 5 requires all previous stories complete for comprehensive testing

---

## Context Preparation Guide

### For Story 1 Tasks (Foundation):
**Required Reading**:
1. Next.js 15 app router documentation
2. React Router v6 documentation
3. Current `web-app/src/` file structure
4. BubbleTea TUI keyboard handling patterns in `app/app.go`

**Required Understanding**:
- Next.js static export configuration
- Client-side routing patterns
- React hooks (useState, useEffect, useCallback)
- Error boundary patterns
- Keyboard event handling

### For Story 2 Tasks (Detail View):
**Required Reading**:
1. `proto/session/v1/session.proto` - Session data structure
2. `server/session_service.go` - StreamTerminal implementation
3. xterm.js documentation
4. ConnectRPC streaming patterns
5. Current `useSessionService` hook

**Required Understanding**:
- PTY streaming from server
- xterm.js Terminal API
- Bidirectional streaming with ConnectRPC
- Git diff format and visualization
- React component lifecycle

### For Story 3 Tasks (Creation Wizard):
**Required Reading**:
1. `ui/overlay/sessionSetup.go` - TUI session creation flow
2. `proto/session/v1/session.proto` - CreateSessionRequest
3. Form validation libraries (zod/yup)
4. Multi-step form patterns

**Required Understanding**:
- Session configuration options
- Git repository detection
- Form state management
- Validation timing (onBlur vs onChange)

### For Story 4 Tasks (Bulk Operations):
**Required Reading**:
1. Current `useSessionService` hook
2. Multi-select UI patterns
3. Optimistic update patterns in React
4. Chart library documentation (recharts)

**Required Understanding**:
- Batch operation patterns
- Selection state management
- Data visualization best practices
- Performance optimization for large lists

### For Story 5 Tasks (Mobile & Accessibility):
**Required Reading**:
1. WCAG 2.1 AA guidelines
2. Responsive design patterns
3. Touch event handling
4. ARIA specification

**Required Understanding**:
- Mobile breakpoints and media queries
- Touch gesture thresholds
- Screen reader behavior
- Color contrast requirements
- Focus management

---

## Integration Checkpoints

### Checkpoint 1: After Story 1
**Validation**:
- Navigation works without full page reloads
- Loading states display correctly during async operations
- Error boundary catches and displays component errors
- Keyboard navigation functional (arrow keys, Enter, /)
- No console errors or warnings

**Rollback Plan**:
- Revert to basic Next.js routing
- Remove keyboard navigation
- Fallback to simple loading/error displays

### Checkpoint 2: After Story 2
**Validation**:
- Session detail pages accessible via URL
- Terminal displays output with correct formatting
- Terminal input sends commands to server
- Diff view shows changes with syntax highlighting
- Info tab displays all session metadata
- Performance: Detail page renders in <200ms

**Rollback Plan**:
- Disable detail view routing
- Keep list view as primary interface
- Store terminal/diff features for later release

### Checkpoint 3: After Story 3
**Validation**:
- Session creation wizard completes successfully
- Form validation prevents invalid submissions
- Path discovery detects git repositories
- Templates load and pre-fill form correctly
- Sessions created via wizard match TUI behavior

**Rollback Plan**:
- Keep existing creation flow
- Disable wizard temporarily
- Fall back to simple form

### Checkpoint 4: After Story 4
**Validation**:
- Multi-select mode activates correctly
- Bulk operations complete without errors
- Advanced filters combine correctly
- Performance dashboard displays accurate stats
- No performance degradation with 100+ sessions

**Rollback Plan**:
- Disable bulk operations
- Remove advanced filtering
- Hide performance dashboard

### Checkpoint 5: After Story 5
**Validation**:
- All pages responsive 320px-2560px
- No horizontal scroll on any viewport
- axe-core reports zero violations
- Screen reader navigation complete
- Touch gestures work on mobile devices
- Lighthouse accessibility score >90

**Rollback Plan**:
- Add mobile warning message
- Focus on desktop experience
- Accessibility improvements as incremental enhancement

### Final Integration Test
**Validation**:
- Complete user flows on desktop, tablet, mobile
- Create session → View detail → Pause → Resume → Delete
- Bulk operations on 50+ sessions
- Real-time updates during session lifecycle
- No memory leaks during 1-hour session
- Performance metrics within targets
- Accessibility compliance verified

---

## Success Criteria (Overall)

- [ ] Web UI feature parity with TUI core features
- [ ] <100ms render time for 100+ session list
- [ ] WCAG 2.1 AA compliance (axe-core zero violations)
- [ ] Responsive design 320px-2560px viewports
- [ ] Zero regressions in existing functionality
- [ ] All async operations have loading/error states
- [ ] Keyboard navigation complete
- [ ] Terminal I/O functional and performant
- [ ] Session creation wizard intuitive
- [ ] Mobile touch gestures implemented
- [ ] Documentation complete for all components
- [ ] Unit test coverage >80%
- [ ] E2E test coverage for critical flows

---

## Implementation Order Recommendation

**Week 1 - Foundation** (Days 1-3):
1. Task 1.1: React Router (2h)
2. Task 1.2: Loading States (2h)
3. Task 1.3: Error Handling (2h)
4. Task 1.4: Keyboard Navigation (3h)

**Week 2 - Detail View Part 1** (Days 4-6):
5. Task 2.1: Detail Layout (2h)
6. Task 2.2: Terminal Display (3h)
7. Task 2.3: Terminal Input (3h)
8. Task 2.4: Diff Visualization (3h) - parallel with 2.2-2.3

**Week 3 - Detail View Part 2 & Creation** (Days 7-10):
9. Task 2.5: Info Tab (2h)
10. Task 3.1: Wizard Form (3h)
11. Task 3.2: Path Discovery (2h)
12. Task 3.3: Templates (2h)

**Week 4 - Advanced Features** (Days 11-12):
13. Task 4.1: Multi-Select (3h)
14. Task 4.2: Advanced Filtering (2h) - parallel
15. Task 4.3: Performance Dashboard (3h) - parallel

**Week 5 - Mobile & Polish** (Days 13-14):
16. Task 5.1: Responsive Layout (3h)
17. Task 5.2: Accessibility (3h)
18. Task 5.3: Touch Gestures (2h)

**Total Effort**: ~47 hours (2.5-3 weeks with testing, review, and integration)

---

## Risk Mitigation

### High-Risk Areas:
1. **Terminal Streaming Performance** - Risk: High-frequency updates cause UI lag
   - Mitigation: Implement virtual scrolling, debounce updates, use canvas rendering

2. **Mobile Performance** - Risk: Complex UI slow on mobile devices
   - Mitigation: Code splitting, lazy loading, service worker caching

3. **Accessibility Compliance** - Risk: Complex interactions hard to make accessible
   - Mitigation: Early accessibility audits, continuous testing, expert review

4. **Real-time Update Conflicts** - Risk: Optimistic updates conflict with server state
   - Mitigation: Version tracking, conflict resolution, pessimistic fallback

### Medium-Risk Areas:
1. **Browser Compatibility** - Different behavior across browsers
   - Mitigation: Cross-browser testing, polyfills, feature detection

2. **Touch Gesture Conflicts** - Gestures interfere with native scrolling
   - Mitigation: Careful threshold tuning, escape hatches, user preferences

3. **Large Session Lists** - Performance degrades with 500+ sessions
   - Mitigation: Virtual scrolling, pagination, aggressive memoization

---

## Testing Strategy

### Unit Tests (80% coverage target):
```bash
# Component tests with React Testing Library
npm test -- --coverage

# Test files:
- SessionList.test.tsx
- SessionCard.test.tsx
- TerminalView.test.tsx
- SessionWizard.test.tsx
- useSessionService.test.ts
```

### Integration Tests:
```typescript
// E2E tests with Playwright
describe("Session Management Flow", () => {
  test("Create session via wizard", async ({ page }) => {
    await page.goto("/sessions/new");
    await page.fill('input[name="title"]', "Test Session");
    await page.fill('input[name="path"]', "/tmp/test-repo");
    await page.click('button:has-text("Next")');
    // ... complete wizard flow
    await page.click('button:has-text("Create Session")');
    await expect(page.locator('.sessionCard')).toContainText("Test Session");
  });

  test("View session details and terminal", async ({ page }) => {
    await page.goto("/");
    await page.click('.sessionCard:first-child');
    await expect(page).toHaveURL(/\/sessions\/[a-z0-9-]+/);
    await expect(page.locator('.terminal')).toBeVisible();
  });
});
```

### Performance Tests:
```typescript
// Performance benchmarks
test("Renders 100 sessions in <100ms", async () => {
  const sessions = generateMockSessions(100);
  const start = performance.now();
  render(<SessionList sessions={sessions} />);
  const duration = performance.now() - start;
  expect(duration).toBeLessThan(100);
});
```

### Accessibility Tests:
```typescript
// Automated a11y testing
test("No accessibility violations", async () => {
  const { container } = render(<App />);
  const results = await axe(container);
  expect(results.violations).toHaveLength(0);
});
```

---

## Documentation Requirements

### Component Documentation:
- Storybook stories for all UI components
- JSDoc comments with prop descriptions
- Usage examples in component files
- Accessibility notes for complex interactions

### API Documentation:
- Update OpenAPI spec if new endpoints added
- Document WebSocket message formats
- ConnectRPC service documentation

### User Documentation:
- Create user guide in `docs/web-ui-guide.md`
- Screenshot-based tutorials for key features
- Keyboard shortcut reference
- Mobile usage tips

---

## Future Enhancements (Post-MVP)

**Not included in current scope but valuable for future iterations**:

1. **Offline Support** - Service worker with offline mode
2. **Collaborative Features** - Real-time collaboration on sessions
3. **Advanced Search** - Full-text search with filters and operators
4. **Session Recording** - Record and replay session activity
5. **Custom Themes** - User-configurable UI themes
6. **Browser Extension** - Quick access from browser toolbar
7. **Mobile Native Apps** - iOS/Android apps with React Native
8. **Session Scheduling** - Cron-like scheduled session operations
9. **Metrics & Analytics** - Detailed usage analytics and insights
10. **Integration Hub** - Connect with GitHub, Jira, Slack, etc.
