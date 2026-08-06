<!-- See CONTRIBUTING.md for the ground rules. -->

## What this changes

## Why

## Checklist

- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...` passes
- [ ] `go test -race -cover ./...` passes
- [ ] No new dependency — `go.mod` still has no `require` block
- [ ] Public API is unchanged, or the change is additive and documented
- [ ] Bug fix includes a test that fails without the fix
- [ ] Secret/`,noexpand` behaviour is untouched, or covered by a new test

## Benchmarks (parser/decoder changes only)

<!-- cd internal/bench && go test -run '^$' -bench . -benchmem -count=5 -->
