# pgstores

## File organization

Each package contains `store.go`, `model.go`, and `query.go`. Split any of these into domain-specific files when they grow large, using the file's base name as a prefix: `model_x.go`, `query_x.go`, etc.

### store.go

- Defines the `Store` struct and its `NewStore` constructor.
- The struct and constructor may carry additional fields, but a `*pgxpool.Pool` is always required.
- Store methods (e.g. `InsertUser()`, `QueryUsers()`) live here as well.
- Group database calls using `pgdb.RunBatch()` or `pgdb.RunBatchTx()` (for transactional work).
- A paginated list method returns both its page and the total number of matching rows. Keep the
  page and count as separate typed queries, but queue both in one `pgdb.RunBatch()` call so they
  share a single database round trip.
- Each method follows this top-down structure:

```go
func (s *Store) GetThing(ctx context.Context, id uuid.UUID) (Thing, error) {
    var thing Thing

    thingQ := getThingQuery(id)

    doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
        if err := thingQ.Queue(ctx, b, &thing); err != nil {
            return fmt.Errorf("get thing: %w", err)
        }
        return nil
    }

    if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
        return Thing{}, err
    }

    return thing, nil
}
```

### model.go

- Defines all model types.

### query.go

- Defines all SQL queries as functions.
- Name them with a `Query` suffix: `createQuery()`, `updateQuery()`, etc.
- Each must return a `pgdb.TypedQuery[T]`, which bundles type information, a scan function, and the result expectation.
- The `Expect` field determines which queue method to use: `ExpectOne` → `.Queue()`, `ExpectMany` → `.QueueMany()`. A mismatch panics at runtime.
- Use `pgx.NamedArgs` for query parameters — `@param_name` placeholders in SQL, collected into a `pgx.NamedArgs` map. Never use positional `$1`, `$2`, etc. Declare the `params` variable before the SQL string.

## Locking

Prefer transaction-level advisory locks when application operations must be serialized. Advisory
locks express the application invariant directly without adding row-lock conflicts to otherwise
independent reads, updates, or foreign-key inserts. Namespace every advisory lock so unrelated
features cannot contend, acquire it inside the transaction it protects, and use a consistent
ordering when an operation needs more than one lock.

Any other locking mechanism must be justified in a nearby comment. The justification must explain
which invariant requires the lock, why an advisory lock is insufficient, why the selected lock mode
is the least restrictive correct mode, and how it interacts with foreign keys and concurrent
queries.

## Testing

- Declare every test file in an external test package named `<package>_test` (for example,
  `package pguser_test`). Store tests import sibling pgstore packages to seed related data; keeping
  tests outside the package under test prevents those setup dependencies from creating import
  cycles.
- Use `pgtest.New(t, ctx)` to get a fresh, isolated `*pgxpool.Pool` per test. Cleanup is registered automatically — do not close the pool manually.
- Each pool costs one connection for the lifetime of the test, so minimize how many pools you open. The right structure depends on whether subtests are isolated from each other:

  **Shared pool** — when subtests are independent of each other's DB state, group them under one `TestStore` function that opens a single pool and passes a `StoreTests` struct to each subtest. This uses one connection regardless of how many subtests there are:

  ```go
  func TestStore(t *testing.T) {
      ctx := context.Background()
      pool := pgtest.New(t, ctx)
      store := NewStore(pool)

      st := &StoreTests{store: store}
      t.Run("insertThing", st.insertThing)
      t.Run("insertThingError", st.insertThingError)
  }

  type StoreTests struct{ store *Store }

  func (st *StoreTests) insertThing(t *testing.T)      { /* ... */ }
  func (st *StoreTests) insertThingError(t *testing.T) { /* ... */ }
  ```

  **Separate pools** — when a test's correctness depends on what is (or isn't) in the database, give it its own pool. `TestStore_QueryUsers` is a good example: its table cases assert a specific set of rows, so it cannot share a pool with `TestStore_InsertUser` — inserts from one would corrupt the other's expectations.

- Use a `seedX` helper (e.g. `seedUser`) for any data inserted as test setup — it makes clear what is precondition vs. what is under test. The helper should return the inserted model so tests can use it in assertions without re-querying.
- DB-generated fields (UUIDs, serials) should be excluded from struct diffs via `cmpopts.IgnoreFields` and asserted non-zero separately.
- Use `cmpopts.EquateApproxTime(time.Minute)` for timestamp comparisons — set `want` to `time.Now()` and let the margin absorb DB round-trip skew.
