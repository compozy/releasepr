# PR Release CLI

`pr-release` automates release preparation for any GitHub repository. It inspects conventional commits, determines the next semantic version, generates changelog entries with `git-cliff`, and orchestrates the Git and GitHub steps needed to open or update a release pull request.

## Overview

The tool wraps the end-to-end release workflow:

- Calculates the next semantic version based on commits since the last tag
- Generates release notes via `git-cliff`
- Creates or updates release branches and tags
- Manages NPM package version bumps when a `tools/` workspace is present
- Produces ready-to-merge release pull requests with rollback support

## Configuration

Configuration is optional for common CI environments. The loader resolves settings in the following order:

1. Environment variables (`GITHUB_REPOSITORY`, `GITHUB_OWNER`, `GITHUB_REPOSITORY_OWNER`, `GITHUB_REPOSITORY_NAME`, `PR_RELEASE_*`, `COMPOZY_RELEASE_*`)
2. YAML configuration file (`.pr-release.yaml` or the legacy name `.compozy-release.yaml`)
3. Git remote discovery (`origin` remote)

Example configuration file:

```yaml
# .pr-release.yaml

github_token: "ghp_your_token"   # Optional; falls back to environment variables
# github_owner and github_repo automatically default to the detected repository
# tools_dir defaults to "tools"
# npm_token is required only when publishing packages

npm_token: "your-npm-token"
tools_dir: "tools"
```

### Environment Variables

| Variable | Description | Required |
| --- | --- | --- |
| `GITHUB_TOKEN`, `PR_RELEASE_GITHUB_TOKEN`, `COMPOZY_RELEASE_GITHUB_TOKEN`, `RELEASE_TOKEN` | GitHub token used for API calls | Only for GitHub operations |
| `GITHUB_REPOSITORY` | `<owner>/<repo>` slug. Highest priority for repository detection | No |
| `GITHUB_OWNER`, `GITHUB_REPOSITORY_OWNER`, `PR_RELEASE_GITHUB_OWNER`, `COMPOZY_RELEASE_GITHUB_OWNER` | Explicit owner override | No |
| `GITHUB_REPOSITORY_NAME`, `PR_RELEASE_GITHUB_REPO`, `COMPOZY_RELEASE_GITHUB_REPO` | Explicit repository name override | No |
| `TOOLS_DIR`, `PR_RELEASE_TOOLS_DIR`, `COMPOZY_RELEASE_TOOLS_DIR` | Directory containing NPM workspaces | No (defaults to `tools`) |
| `NPM_TOKEN`, `PR_RELEASE_NPM_TOKEN`, `COMPOZY_RELEASE_NPM_TOKEN` | Token for publishing to NPM | Required for publishing |

## Commands

The CLI is built with Cobra and exposes the following commands:

| Command | Description |
| --- | --- |
| `pr-release` | Run the full release orchestration workflow |
| `dry-run` | Execute release steps without pushing or opening PRs |
| `version` | Print build metadata |

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
  arm64|aarch64) ARCH="arm64" ;;
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

> Prefer not to use `jq`? Head to the [Releases](https://github.com/compozy/releasepr/releases) page,
> pick the desired tag manually, and substitute it for `${VERSION}` in the snippet above.

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
          go-version: "1.25.1"
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

| Secret | Purpose |
| --- | --- |
| `GORELEASER_KEY` | GoReleaser Pro license key |
| `AUR_KEY` | AUR publishing |
| `NPM_TOKEN` | Publish packages to npm |

## GitHub Actions

The repository ships with `ci.yml` for validation and `release.yml` for automated release pull requests, dry-run verification, and production releases. These workflows assume the same environment variables described above and do not embed repo-specific defaults.

---

For more details on contributing or extending the CLI, inspect the orchestrator and use case packages, and review the `.cursor/rules/` guidelines that define coding and testing standards for the project.
