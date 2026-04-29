# go-finance

## Module
github.com/hackmajoris/go-finance

## Structure
- `cmd/go-finance/` — single binary entry point; keep thin, wire into `pkg/`
- `pkg/` — all business logic; each package independently testable

## Adding a New Package
1. Create `pkg/<n>/`
2. Define the main type and a `New()` constructor with functional options
3. Add `mocks/` sub-package if the package exposes an interface used by others
4. Add `<n>_test.go` using the `_test` package suffix

## Testing
- Run: `go test -race ./...`
- Prefer table-driven tests
- Use `testdata/` for fixtures
- Mocks live in `pkg/<n>/mocks/`

## Conventions
- Functional options: `type Option func(*Client)`
- Sentinel errors: `var ErrNotFound = errors.New("not found")`
- Wrap errors: `fmt.Errorf("doing X: %w", err)`
- `main()` calls `run(args, out)` — keeps entry point testable
