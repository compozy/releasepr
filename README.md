# PR Release CLI

`pr-release` automates release preparation for any GitHub repository. It inspects conventional commits, determines the next semantic version, generates changelog entries with `git-cliff`, and orchestrates the Git and GitHub steps needed to open or update a release pull request.

## Overview

The tool wraps the end-to-end release workflow:

- Calculates the next semantic version based on commits since the last tag
- Generates changelogs via `git-cliff`
- Generates current release bodies and historical release notes from scoped changelogs plus optional `.release-notes/*.md` content
- Creates or updates release branches and tags
- Manages NPM package version bumps when a `tools/` workspace is present
- Produces ready-to-merge release pull requests with rollback support
- Validates explicit beta, stable, and legacy release inputs as one machine-readable plan
- Renders explicit release bodies from the exact predecessor-to-commit Git range

## Agent Skill

This repository ships an official agent skill at [`skills/releasepr`](skills/releasepr)
that teaches AI coding agents how to set up, run, and troubleshoot pr-release
in a consuming project.

Install it into your agent skills directory with the
[`skills`](https://www.npmjs.com/package/skills) CLI:

```bash
npx skills add https://github.com/compozy/releasepr --skill releasepr
```

## Configuration

Configuration is optional for common CI environments. The loader resolves settings in the following order:

1. Environment variables (`GITHUB_REPOSITORY`, `GITHUB_OWNER`, `GITHUB_REPOSITORY_OWNER`, `GITHUB_REPOSITORY_NAME`, `PR_RELEASE_*`, `COMPOZY_RELEASE_*`)
2. YAML configuration file (`.pr-release.yaml` or the legacy name `.compozy-release.yaml`)
3. Git remote discovery (`origin` remote)

Example configuration file:

```yaml
# .pr-release.yaml

github_token: "ghp_your_token" # Optional; falls back to environment variables
# github_owner and github_repo automatically default to the detected repository
# tools_dir defaults to "tools"
# npm_token is required only when publishing packages

npm_token: "your-npm-token"
tools_dir: "tools"
```

### Environment Variables

| Variable                                                                                             | Description                                                      | Required                   |
| ---------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- | -------------------------- |
| `GITHUB_TOKEN`, `PR_RELEASE_GITHUB_TOKEN`, `COMPOZY_RELEASE_GITHUB_TOKEN`, `RELEASE_TOKEN`           | GitHub token used for API calls                                  | Only for GitHub operations |
| `GITHUB_REPOSITORY`                                                                                  | `<owner>/<repo>` slug. Highest priority for repository detection | No                         |
| `GITHUB_OWNER`, `GITHUB_REPOSITORY_OWNER`, `PR_RELEASE_GITHUB_OWNER`, `COMPOZY_RELEASE_GITHUB_OWNER` | Explicit owner override                                          | No                         |
| `GITHUB_REPOSITORY_NAME`, `PR_RELEASE_GITHUB_REPO`, `COMPOZY_RELEASE_GITHUB_REPO`                    | Explicit repository name override                                | No                         |
| `TOOLS_DIR`, `PR_RELEASE_TOOLS_DIR`, `COMPOZY_RELEASE_TOOLS_DIR`                                     | Directory containing NPM workspaces                              | No (defaults to `tools`)   |
| `NPM_TOKEN`, `PR_RELEASE_NPM_TOKEN`, `COMPOZY_RELEASE_NPM_TOKEN`                                     | Token for publishing to NPM                                      | Required for publishing    |

## Commands

The CLI is built with Cobra and exposes the following commands:

| Command        | Description                                          |
| -------------- | ---------------------------------------------------- |
| `add-note`     | Create a custom release note entry                   |
| `pr-release`   | Run the full release orchestration workflow          |
| `dry-run`      | Execute release steps without pushing or opening PRs |
| `plan`         | Validate and emit an explicit release plan           |
| `release-body` | Render a release body from an explicit Git range     |
| `version`      | Print build metadata                                 |

Run `go run . <command> --help` for detailed flags.

## Example Usage

### CLI (prebuilt release)

```bash
# Fetch the latest tag from the releases API (requires jq)
VERSION=$(curl -sSf https://api.github.com/repos/compozy/releasepr/releases/latest | jq -r .tag_name)

# Map local OS/Arch to the archive naming used by GoReleaser
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="x86_64" ;;
  amd64) ARCH="x86_64" ;;
  arm64 | aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" && exit 1 ;;
esac

# Download and extract the published binary
curl -L "https://github.com/compozy/releasepr/releases/download/${VERSION}/pr-release_${VERSION}_${OS}_${ARCH}.tar.gz" \
  -o pr-release.tgz
tar -xzf pr-release.tgz

# Verify the installation
./pr-release/pr-release version

# Run the orchestrator
GITHUB_TOKEN=... ./pr-release/pr-release pr-release --dry-run --enable-rollback
```

### Add a custom release note

```bash
./pr-release/pr-release add-note \
  --title "Shared layout package" \
  --type feature
```

Use `--body` to provide the markdown inline instead of opening `$EDITOR`.

The generated files live in `.release-notes/` and are archived automatically to
`.release-notes/archive/vX.Y.Z/` after a release branch is prepared.

> Prefer not to use `jq`? Head to the [Releases](https://github.com/compozy/releasepr/releases) page,
> pick the desired tag manually, and substitute it for `${VERSION}` in the snippet above.

### Plan an explicit release

Use `plan` when a workflow supplies the release ref, version, and channel instead
of deriving them from commits or branch names:

```bash
pr-release plan \
  --ref refs/heads/main \
  --version 0.3.0-beta.1 \
  --channel beta
```

The command is read-only. It requires the ref to resolve to the checked-out
`HEAD`, rejects an existing local or remote tag, derives `v0.3.0-beta.1` once,
and returns the version, tag, commit, predecessor tag, Git range, and
publication policy as JSON. The version must be strict SemVer without a
leading `v`. Beta plans consider prerelease tags when selecting the nearest
predecessor on the first-parent release line; stable and legacy plans consider
stable tags only. `initial_release` is true only when that line has no matching
predecessor.

For an explicit retry after tag creation, add `--allow-existing-tag`. Recovery
is accepted only when the existing local or remote tag is annotated and
resolves to the planned commit. The resumed tag is excluded when selecting the
predecessor. Without the flag, any existing tag remains an error.

For GitHub Actions, write deterministic outputs directly to the step output
file:

```yaml
- name: Plan release
  id: release_plan
  run: >-
    go run "${{ env.PR_RELEASE_MODULE }}" plan
    --ref "${{ inputs.ref }}"
    --version "${{ inputs.version }}"
    --channel "${{ inputs.channel }}"
    --format github >> "$GITHUB_OUTPUT"
```

`beta` produces GitHub prerelease intent, npm tag `beta`, and a Homebrew skip.
`stable` and `legacy` produce stable/latest intent, npm tag `latest`, and a
Homebrew publish. The consuming workflow creates the tag and performs the
publishing from these outputs; it must not infer or normalize them again.

Render the body before creating the tag and pass the planned range unchanged:

```bash
pr-release release-body \
  --tag "$RELEASE_TAG" \
  --range "$RELEASE_GIT_RANGE" > release-body.md
```

The command writes Markdown only to stdout. `git-cliff` receives the exact
range, and custom notes are limited to files introduced in that range for the
target core version. When `release_initial` is true, pass `--initial` instead
of `--range`; omitting both selectors fails closed.

### From source (optional)

```bash
go install github.com/compozy/releasepr@latest
~/go/bin/releasepr version
```

### GitHub Actions

```yaml
name: Release Dry Run

on:
  workflow_dispatch:

jobs:
  release:
    runs-on: ubuntu-latest
    env:
      GITHUB_TOKEN: ${{ secrets.RELEASE_TOKEN }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.9"
      - name: Run pr-release dry run
        run: go run . pr-release --dry-run --ci-output
```

## Architecture Highlights

- **`cmd/`** – CLI commands and dependency injection container
- **`internal/orchestrator/`** – High-level workflows coordinating repositories and services
- **`internal/usecase/`** – Business logic for atomic release steps
- **`internal/repository/`** – Git, GitHub, filesystem adapters
- **`internal/service/`** – Integrations such as `git-cliff`, `npm`, and GoReleaser
- **`internal/config/`** – Configuration loading, validation, and repository detection

## Environment Reference

The release workflow relies on additional secrets when running in CI:

| Secret           | Purpose                    |
| ---------------- | -------------------------- |
| `GORELEASER_KEY` | GoReleaser Pro license key |
| `AUR_KEY`        | AUR publishing             |
| `NPM_TOKEN`      | Publish packages to npm    |

## GitHub Actions

The repository ships with `ci.yml` for validation and `release.yml` for automated release pull requests, dry-run verification, and production releases. These workflows assume the same environment variables described above and do not embed repo-specific defaults.

Production releases are published from `RELEASE_BODY.md`. The release workflow wires GoReleaser with:

- `--release-notes=RELEASE_BODY.md`
- `--release-header-tmpl=.goreleaser.release-header.md.tmpl`
- `--release-footer-tmpl=.goreleaser.release-footer.md.tmpl`

`RELEASE_BODY.md` contains only the current release section. `RELEASE_NOTES.md` is the committed historical document and prepends the current release while preserving older release sections.

---

For more details on contributing or extending the CLI, inspect the orchestrator and use case packages, and review the `.cursor/rules/` guidelines that define coding and testing standards for the project.
