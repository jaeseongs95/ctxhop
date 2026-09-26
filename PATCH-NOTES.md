# CtxHop 0.2.0-gui.1 local patch

Upstream base: `v0.2.0`, commit `b8a18e917a3e93b2e961e8e12c82a4953a98779d`.

The GUI's clean preview can become stale before a separate resume reads remote metadata again. Upstream resume can then apply filtered settings, MCP configuration and skills after restoring the native session. This patch adds a request-local `--no-environment` flag at the shared resume parser and execution path used by both `resume` and `session resume`.

With the flag set, resume skips the environment conflict gate and never calls `applyEnvironmentComponents`. The JSON result includes `environmentSkipped: true`, including preview responses and native restores with no environment metadata. Environment preview remains read-only and may still report observed remote components. The existing workspace context flag and native restore behavior remain separate.

Production changes:

- `cmd/ctxhop/resume.go`: flag, report field, conflict gate and environment apply guard.
- `cmd/ctxhop/discovery.go`: flag advertised for both command spellings.

Regression changes:

- `cmd/ctxhop/resume_test.go`: parsing, explicit false, positional flag normalization and both help paths.
- `cmd/ctxhop/resume_no_environment_test.go`: actual encrypted remote and native restore integration for Claude Code and Codex. It creates a clean preview, adds applicable settings/MCP/skill metadata to the isolated remote, checks the changed preview, checks preview JSON with the flag, restores with the flag, verifies native records and verifies no environment targets or environment backups exist. A control using the same bodies without the flag writes all three component targets through the actual providers. A project settings override then creates an environment conflict; the ordinary restore rejects it, while the flagged restore succeeds and preserves all environment file bytes.

All fixture directories are descendants of `D:\Go\temp`, following the copied `AGENTS.md`. No actual user session, config, encryption key or remote store was used. The source fixture uses only native restore, with no custom session migration or session database conversion.

## Build and verification

The build used the official portable `go1.27.1.windows-amd64.zip` from `https://go.dev/dl/`, with metadata obtained from `https://go.dev/dl/?mode=json` and the ZIP SHA-256 checked before extraction:

`a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d`

The toolchain and build caches are scoped to `C:\Users\Lenovo\AppData\Local\Temp\ctxhop-go-gui1`. The reusable regression runner keeps its GOPATH and GOCACHE under `D:\Go\temp\ctxhop-go-regression-work` regardless of the supplied executable path; GOTOOLCHAIN is `local`. There was no system Go installation, global PATH edit or official CtxHop installation.

Build command, executed from this source directory with the same process-local Go environment as the runner:

```powershell
go build -trimpath -ldflags '-s -w -X main.version=0.2.0-gui.1 -X main.commit=b8a18e9+gui.1 -X main.date=2026-09-26' -o 'D:\세션연동\ctxhop-gui\bin\ctxhop.exe' ./cmd/ctxhop
```

Re-run verification with an explicit local Go executable (Go 1.26 or newer is required by go.mod):

```powershell
& '.\Run-Regression.ps1' -GoExe 'C:\Users\Lenovo\AppData\Local\Temp\ctxhop-go-gui1\go\bin\go.exe'
```

`Run-Regression.ps1` passed on 2026-09-26. Raw stdout is saved in `verification-results/targeted-regression.txt` and `verification-results/relevant-suites.txt`. `go vet ./cmd/ctxhop` exited 0 with no output; `verification-results/vet.txt` is empty. The passing suites were cmd/ctxhop, internal/environment, internal/adapter, internal/syncflow and internal/syncer.

Binary: `D:\세션연동\ctxhop-gui\bin\ctxhop.exe`; size 11,735,040 bytes. Version stdout: `ctxhop 0.2.0-gui.1`. Both `help resume` and `help session resume` list `--no-environment`.

Binary SHA-256:

`a1702ce1839af90c0ddb87e7c07f1be7899be8ebdd9117fe680d2ec9739c233d`

The machine-readable metadata is in `binary-manifest.json`. The adjacent `../ctxhop-no-environment.patch` contains the production and regression source diffs against the pristine upstream source, with relative paths and LF line endings. `git -c core.autocrlf=false apply --check` passed against the pristine comparison files.

## Scope and limitations

This patch protects the filtered environment component application path on both native adapters. The GUI must pass `--no-environment` on the actual restore, verify `environmentSkipped` is exactly JSON boolean true, and use the pinned local binary. The flag does not disable an explicitly requested `--workspace` restore, which is an independent upstream operation. The GUI does not request workspace restore.

The verified fixtures invoke the actual in-process command entry point (`runSessionWithStreams`) using a directory backend, encrypted local test material, real native adapters and real filesystem environment providers. They do not stub or spy on environment application. The built executable was separately smoke-tested for version and both help paths. No actual user's restore was executed; live remote behavior and the GUI wrapper's binary pinning are separate integration checks. The portable Go toolchain and caches are verification dependencies and must not be bundled with the GUI.
