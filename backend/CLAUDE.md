# Backend development

## Error wrapping

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

## Testing conventions

### Table tests

- Name the slice `tests` and the loop variable `tt`.
- The `name` field describes only what differentiates the case — assume the function under test is known.
- No blank line between the slice declaration and the `for` loop.
- Never use a `wantErr bool` field. Split success and error cases into separate functions named `TestXyz` and `TestXyz_error`.

### Error reporting

- Include the function name, relevant inputs, and actual vs. expected values in every failure message.
- Format: `Func(<input>) = <got>, want <want>`.
- Print got before want.
- Use `t.Errorf` when the test can still make further assertions after the failure.
- Use `t.Fatalf` only when the test cannot meaningfully continue — typically failed setup or an unexpected error that leaves no result to check.

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

## Schema code generation

Proto schemas live in `schemas/` at the repo root. Generated pb files are committed under
`internal/api/grpc/internal/pb/`. To regenerate them after editing a `.proto` file, run
`make generate` from the repo root.

## Mocking interfaces

Use [moq](https://github.com/matryer/moq) to generate mocks. Place a `go:generate` directive on the file that defines the interface, then run `go generate ./...` to produce the mock:

```go
//go:generate moq -rm -fmt goimports -out storer_moq_test.go . Storer:MockedStorer
```

Never write mocks by hand.
