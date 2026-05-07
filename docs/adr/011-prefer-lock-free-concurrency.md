# ADR 011: Prefer Lock-Free Concurrency Techniques

## Status
Accepted

## Context
As the Stapler Squad codebase grows in complexity, particularly with background polling (Review Queue), real-time terminal streaming (Control Mode), and multi-session management, lock contention on global state (like `Instance.stateMutex`) has become a performance bottleneck and a source of deadlocks. 

While we have instrumented the codebase with `github.com/linkdata/deadlock` to detect runtime deadlocks, the long-term architectural goal is to reduce the footprint of mutual exclusion in favor of lock-free techniques where appropriate.

## Decision
We will prefer lock-free concurrency techniques for high-contention or simple state synchronization tasks. In Go, this is primarily achieved through atomic operations and specialized data structures.

### Core Mechanisms
- **Atomic Operations**: Utilize `sync/atomic` for low-level functions (Add, Load, Store, Swap) to manage simple counters, flags, and pointers.
- **Compare-And-Swap (CAS)**: Use CAS as the building block for lock-free updates, ensuring a value is only updated if it matches an expected state.
- **Memory Barriers**: Rely on Go's memory model and atomic operations to ensure visibility across CPU cores.

### Common Techniques to Employ
- **CAS Loops**: Retrying operations until success to ensure progress without blocking.
- **Copy-on-Write (Immutable State)**: Atomically updating a pointer to a new immutable version of a data structure, allowing readers to access the old version concurrently without locks.
- **Disjoint Memory Access**: Structuring data (e.g., per-session or per-goroutine state) to eliminate shared writable memory where possible.

### Native & Third-Party Tools
- **sync.Map**: Use for read-heavy maps or when keys are mostly stable.
- **Specialized Libraries**: For high-performance FIFO queues or concurrent hashmaps, we will evaluate:
  - `ahrav/go-lockfree-queue`
  - `cornelk/hashmap`
  - `ajitpratap0/nebula/pkg/lockfree`

## Consequences
- **Positive**: Reduced lock contention, improved throughput in the daemon/server layers, and elimination of a large class of deadlock risks.
- **Negative**: Increased complexity in implementation and debugging. Lock-free logic is notoriously difficult to get right and requires rigorous testing.
- **Neutral**: We will maintain `linkdata/deadlock` for all remaining mutexes as a safety net.

## Guidelines: When to Use
| Scenario | Recommended Approach |
| :--- | :--- |
| **Simple Counters/Flags** | Use `sync/atomic` for speed and simplicity. |
| **High Contention** | Evaluate lock-free structures to avoid thread suspension. |
| **Complex State Transitions** | Use `sync.Mutex` or Channels; lock-free logic for complex state machines is high-risk. |
| **Read-Heavy Maps** | Consider `sync.Map` or a specialized lock-free map. |
