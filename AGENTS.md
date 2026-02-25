## Cursor Cloud specific instructions

### Project overview
Zeonica is a cycle-accurate CGRA / wafer-scale accelerator simulator written in Go 1.24. It is a pure Go module with no Docker, databases, or external services. See `README.md` for full architecture details.

### Required setup
- Go 1.24+ (pre-installed in cloud VM).
- Git submodule `test/Zeonica_Testbench` must be initialized: `git submodule update --init --recursive`.
- `$(go env GOPATH)/bin` must be on `PATH` (already appended to `~/.bashrc`).

### Key commands
| Task | Command |
|---|---|
| Build | `go build ./...` |
| Lint | `go vet ./...` |
| Tests (standard) | `go test ./...` |
| Tests (Ginkgo BDD) | `ginkgo -r` |
| Verify AXPY kernel | `go run ./verify/cmd/verify-axpy` |
| Verify histogram kernel | `go run ./verify/cmd/verify-histogram` |
| Verify FIR kernel | `go run ./verify/cmd/verify-fir` |

### Gotchas
- The globally installed Ginkgo CLI version may differ from the `go.mod` pinned version (v2.20.2). A version mismatch warning is printed but tests still pass. To use the exact module version: `go run github.com/onsi/ginkgo/v2/ginkgo -r`.
- The CI workflow (`go test ./...` + `ginkgo -r`) does **not** initialize submodules in the checkout step, but all testbench tests that depend on submodule YAML files will fail if the submodule is missing.
- Python `requirements.txt` is only needed for optional visualization/debugger tools, not for core build/test.
- This project has no UI; it is a CLI/library. Manual GUI testing is not applicable.
