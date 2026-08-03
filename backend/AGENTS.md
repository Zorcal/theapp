# Backend development

## Error handling

### Wrapping

Wrap errors with `fmt.Errorf` when the added context makes the log line meaningfully easier to debug — when without it a
reader couldn't tell where in the call stack the error came from or what was being attempted.

A well crafted error message tells a story. Error messages are always open for concatenation — a caller will prepend
their own context. The message should be a short verb phrase describing what the code was trying to do. Avoid words like
`failed`, `cannot`, `unable to`, `error while`, `could not` when wrapping erors — they add noise and read badly when
chained.

Good chain:

```
track parcel location: fetch order status: failed to connect to db
```

Bad chain:

```
error while tracking location: unable to fetch order status: DB connection failed
```

### Style

Assign and check an error in the same `if` statement when no other values are needed outside the block. Split into two statements only when the result value is used after the check.

Never end a function by returning the result of a call directly (`return pgdb.RunBatch(...)`, `return c.transactor.RunTx(...)`). Always check it explicitly and return `nil` on the success path, even when there's nothing else to do:

```go
// Good
if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
    return err
}
return nil

// Bad
return pgdb.RunBatch(ctx, s.pool, doInBatch)
```

This keeps every function's error handling shape the same regardless of whether a case is later added that needs to do something after the call succeeds — that case doesn't force a restructure of an existing bare `return`.

## Code organisation

Order functions by importance: exported and primary functions first, helpers at the bottom. Group functions by relevance.

Keep function and method signatures and calls on one line when they fit comfortably. Split them
across lines only when the single line would become extremely long; do not default to placing each
argument or parameter on its own line.

## Naming

- Length is not a virtue in a name; clarity of expression is.
- Prefer minimum-length, maximum-information names, then let context fill in the rest.
- Prefer established domain abbreviations in variable names, such as `org` instead of
  `organization` and `sess` instead of `session`. Keep the full words in exported API, type, and
  method names where they describe domain concepts.
- Boolean variables should read as predicates. Prefix them with `is` or `has` when that makes the
  condition clearer (`isMember`, `hasPermission`). A state already expressed as an adjective or
  predicate does not need a forced prefix (`rateLimited`, `exists`), and conventional comma-`ok`
  values remain `ok`. Prefer a concise name such as `exists` when the small scope makes its subject
  obvious; don't lengthen it merely to encode context already visible next to the declaration.

## Imports

Avoid import aliases. Only alias an import when two imported packages would otherwise have the same name and both are used in the same file.

## Testing conventions

Always run the full test suite with `make test` from `backend/`, including when verifying a change isolated to one
package. Do not run package-specific or test-specific commands.

### Subtests

- The subtest name describes only what differentiates the case — assume the function under test is known. Inside a `TestFoo_error` function every subtest already produces an error, so don't restate that in the name (`"bar returns ErrBar"`, `"baz fails"`, `"qux errors out"`) — name it after only the differentiating cause (`"bar"`, `"baz"`, `"qux"`).

### Table tests

Table tests are a subtest structured as a data-driven loop; the naming rule above still applies to the `name` field.

- Name the slice `tests` and the loop variable `tt`.
- Each case has a `name` field used as the subtest name (`t.Run(tt.name, ...)`) — exceptions are rare.
- No blank line between the slice declaration and the `for` loop.
- Never use a `wantErr bool` field. Split success and error cases into separate functions named `TestXyz` and `TestXyz_error`.
- Each test case struct field on its own line, using named fields.

### Error reporting

- Include the function name, relevant inputs, and actual vs. expected values in every failure message.
- Format: `Func(<input>) = <got>, want <want>`.
- Print got before want.
- Use `t.Errorf` when the test can still make further assertions after the failure.
- Use `t.Fatalf` only when the test cannot meaningfully continue — typically failed setup or an unexpected error that leaves no result to check.
- Outside table tests, declare `got` and `want` together — `if got, want := f(x), y; got != want { t.Errorf("f(%v) = %v, want %v", x, got, want) }` — and use `want` in the message instead of hardcoding the expected value again.

### Mocks

- Use mock callbacks to provide configured results and errors. Don't assert callback arguments or examine mock call
  history merely to verify that values were forwarded between collaborators; those are white-box tests of the current
  call graph, not observable behavior. Prefer assertions on returned values, errors, and persisted state, and use
  integration tests when behavior depends on values crossing a layer boundary. Assert mock interactions only in the
  rare case where the interaction itself is the contract, such as a required external side effect with no other
  observable result.

```go
// Good
func TestCalculate(t *testing.T) {
    tests := []struct {
        name string
        in   int
        want int
    }{
        {"zero", 0, 0},
        {"positive", 5, 25},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Calculate(tt.in)
            if got != tt.want {
                t.Errorf("Calculate(%d) = %d, want %d", tt.in, got, tt.want)
            }
        })
    }
}

func TestCalculate_error(t *testing.T) {
    tests := []struct {
        name string
        in   int
    }{
        {"too large", math.MaxInt},
        {"invalid range", -1000},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if _, err := Calculate(tt.in); err == nil {
                t.Errorf("Calculate(%d) error = nil, want error", tt.in)
            }
        })
    }
}
```

## Comments

This applies to inline code comments and to any other documentation — READMEs, design docs, everything — not just Go comments.

- Don't restate what code already makes obvious, including a fact already visible in the same declaration (a field the struct simply doesn't have, a case the switch doesn't handle). If removing the comment wouldn't confuse a reader, remove it.
- Only compare one thing to another when the comparison itself is the non-obvious fact worth recording — e.g. two things that look alike but are deliberately handled differently. Don't reach for a comparison to state something that already stands on its own without one; that forces the reader to hold a second definition in mind for no payoff, and it's usually the first part to go wrong when either side changes later.
- Don't cite a file path, doc path, or another function/test name as the reason something is true ("see foo.md", "see TestBar for why"). If the reasoning is worth having, state it inline. A citation rots the moment its target moves, renames, or changes, and it gives the reader nothing the fact itself doesn't already say.
- Don't write comments that explain a decision by narrating the conversation that led to it ("we agreed to do X", "per your request", "as discussed, this is intentional"). State the constraint plainly, or let the code speak for itself.
- When a correction (from the user, or your own re-check) changes what you believe is true, write the resulting text as if you'd known the correct fact from the start. The exchange that revealed it — what was assumed, what changed, why — is not part of the domain and must leave no trace in the doc. Narrating that exchange is the usual way this mistake shows up.
- Avoid comments that go stale as soon as the code around them changes. Describe purpose, not the current shape of the code.

## Sentinel errors documention

Document sentinel errors in godoc using a `// Returns [ErrFoo] if ...` line on the function or method:

```go
// UserByID returns the user with the given ID.
// Returns [mdl.ErrNotFound] if no user with that ID exists.
func (c *Core) UserByID(ctx context.Context, id uuid.UUID) (mdl.User, error)
```

Interface methods must also declare what sentinels they require from implementations:

```go
type Storer interface {
    // UserByExternalID returns the user with the given external ID.
    // Returns [sql.ErrNoRows] if no such user exists.
    UserByExternalID(ctx context.Context, id uuid.UUID) (pguser.User, error)
}
```

## Sets

Use pkg/set for sets instead of `map[string]struct{}`.

## Linting

Run `golangci-lint run ./...` from `backend/` after finishing a change. Fix all issues before considering the task done.

## Schema code generation

Proto schemas live in `schemas/` at the repo root. Generated pb files are committed under
`internal/api/grpc/internal/pb/`. To regenerate them after editing a `.proto` file, run
`make generate` from the repo root.
