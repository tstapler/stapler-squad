# Interface Pollution — Catch Leaky Abstractions Before They Merge

This repo is written almost entirely with Claude Code. LLMs default to Java/Spring-shaped
abstractions — interfaces-first, Repository/Manager/Service layering, getter/setter pairs —
which don't mesh with Go's data model and make code harder to maintain, not easier. Review
every new type/interface an LLM introduces against this checklist before merging.

## The 6 Smells to Detect

1. **Speculative interface** — an interface with exactly one implementation and no near-term
   second one.
2. **Interface defined next to its implementation** — declared in the same package as the
   concrete type that satisfies it, rather than in the package that consumes it.
3. **No-op getter/setter** — an accessor method that only reads or writes a field with no
   validation, computation, or invariant enforcement.
4. **Forwarding-only wrapper** — a `Manager`/`Handler`/`Processor`/`Service` type whose
   methods each call straight through to another type with no added behavior.
5. **Unjustified generic** — a generic function or type used at a single call site that a
   concrete type or a plain loop would express more clearly.
6. **Struct-wraps-struct-wraps-struct** — multiple layers of embedding/wrapping where each
   layer re-exposes the layer below without adding new exported behavior.

## The Correct Pattern for Each

1. Use the concrete type directly (e.g. `*postgresUserStore`) until a second real
   implementation exists or is imminent:
   ```go
   type postgresUserStore struct{ db *sql.DB }
   func NewUserStore(db *sql.DB) *postgresUserStore { return &postgresUserStore{db: db} }
   func (s *postgresUserStore) Get(ctx context.Context, id string) (*User, error) { ... }
   ```

2. Define the interface where it's consumed, scoped to only the methods that consumer needs:
   ```go
   // service/user_service.go — consumer package
   type UserStore interface {
       Get(ctx context.Context, id string) (*User, error)
   }
   // postgres package just happens to satisfy it — no "implements" declaration needed
   ```

3. Export the field directly and let callers use it:
   ```go
   type User struct {
       Name string
   }
   u.Name = "Alice"
   ```

4. Call the wrapped type directly; only add a wrapping type when it contributes real
   behavior at that layer (shown here: caching):
   ```go
   type cachingUserStore struct {
       next  UserStore
       cache *lru.Cache
   }
   func (c *cachingUserStore) Get(ctx context.Context, id string) (*User, error) {
       if u, ok := c.cache.Get(id); ok {
           return u.(*User), nil
       }
       u, err := c.next.Get(ctx, id)
       if err != nil {
           return nil, err
       }
       c.cache.Add(id, u)
       return u, nil
   }
   ```

5. Write the concrete version first; generalize only once 2+ real call sites need the
   identical logic:
   ```go
   func ActiveUserIDs(users []User) []string {
       ids := make([]string, 0, len(users))
       for _, u := range users {
           if u.Active {
               ids = append(ids, u.ID)
           }
       }
       return ids
   }
   ```

6. Collapse to one struct; each field or layer must earn its place by adding behavior:
   ```go
   type Session struct {
       ID        string
       StartedAt time.Time
       mu        sync.Mutex
       state     SessionState
   }
   ```

## Why

Go's idiom is the inverse of the layered-abstraction style Java/Spring-heavy training data
over-represents. Rob Pike: "Don't design with interfaces, discover them." Interfaces belong
in the *consumer* package, not the implementer's — the opposite of Java convention. See the
`go-development` skill for the full idiom set this checklist complements.
