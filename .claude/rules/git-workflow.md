# Git Workflow

- Never commit to `main`. Branch first: `feat/`, `fix/`, `chore/`, `docs/`, `refactor/`,
  `test/` + short description.
- On a feature branch, commit as you go without asking. This is the commit authority the Beads
  block leaves to the repository. Stage files by name.
- Do not push, open a PR, merge, or run `bd dolt push` unless asked.
- Commit subjects use Conventional Commits.
- Before every commit, all four must be clean:
  `test -z "$(gofmt -l .)" && go vet ./... && golangci-lint run && go test ./...`
  The pre-commit hook enforces it. CI runs the same checks on Linux and macOS.
- Work lands on `main` through a PR with green CI, squash-merged. While the repository has no
  remote, stop at a clean, committed branch and report.
