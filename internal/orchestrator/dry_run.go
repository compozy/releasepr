// internal/orchestrator/dry_run.go
package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/compozy/releasepr/internal/logger"
	"github.com/compozy/releasepr/internal/repository"
	"github.com/compozy/releasepr/internal/service"
	"github.com/spf13/afero"
	"go.uber.org/zap"
)

const (
	githubActionsTrue     = "true"
	envGithubIssueNumber  = "GITHUB_ISSUE_NUMBER"
	envGithubEventPath    = "GITHUB_EVENT_PATH"
	envGithubHeadRef      = "GITHUB_HEAD_REF"
	envGithubSHA          = "GITHUB_SHA"
	envGithubActions      = "GITHUB_ACTIONS"
	metadataJSONPath      = "dist/metadata.json"
	artifactTypeArchive   = "Archive"
	releaseHeaderTmplPath = ".goreleaser.release-header.md.tmpl"
	releaseFooterTmplPath = ".goreleaser.release-footer.md.tmpl"
)

// DryRunConfig holds configuration for the dry-run orchestrator
type DryRunConfig struct {
	CIOutput bool // Output in CI format
	DryRun   bool // Always true for this orchestrator, but for consistency
}

// DryRunOrchestrator orchestrates the dry-run validation process
type DryRunOrchestrator struct {
	gitRepo       repository.GitExtendedRepository
	githubRepo    repository.GithubExtendedRepository
	cliffSvc      service.CliffService
	goreleaserSvc service.GoReleaserService // Assuming this exists in service/goreleaser.go
	fsRepo        afero.Fs
}

// NewDryRunOrchestrator creates a new DryRunOrchestrator
func NewDryRunOrchestrator(
	gitRepo repository.GitExtendedRepository,
	githubRepo repository.GithubExtendedRepository,
	cliffSvc service.CliffService,
	goreleaserSvc service.GoReleaserService,
	fsRepo afero.Fs,
) *DryRunOrchestrator {
	return &DryRunOrchestrator{
		gitRepo:       gitRepo,
		githubRepo:    githubRepo,
		cliffSvc:      cliffSvc,
		goreleaserSvc: goreleaserSvc,
		fsRepo:        fsRepo,
	}
}

func (o *DryRunOrchestrator) logger(ctx context.Context) *zap.Logger {
	return logger.FromContext(ctx).Named("orchestrator.dry_run")
}

// Execute runs the dry-run validation
func (o *DryRunOrchestrator) Execute(ctx context.Context, cfg DryRunConfig) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultWorkflowTimeout)
	defer cancel()
	if err := o.stepValidateChangelog(ctx, cfg); err != nil {
		return err
	}
	if err := o.stepRunGoReleaser(ctx, cfg); err != nil {
		return err
	}
	_, err := o.stepExtractVersion(ctx, cfg)
	if err != nil {
		return err
	}
	// NPM validation of tools/ removed from dry-run pipeline
	if os.Getenv(envGithubActions) == githubActionsTrue {
		if err := o.stepCommentPR(ctx, cfg); err != nil {
			return err
		}
	} else {
		o.logStatus(ctx, cfg.CIOutput, "Dry-run completed. Review required.")
	}
	o.logStatus(ctx, cfg.CIOutput, "## ✅ Dry-Run Completed Successfully")
	return nil
}

// stepValidateChangelog validates git-cliff changelog generation
func (o *DryRunOrchestrator) stepValidateChangelog(ctx context.Context, cfg DryRunConfig) error {
	o.logStatus(ctx, cfg.CIOutput, "### 📝 Validating Changelog Generation")
	if err := o.validateCliff(ctx); err != nil {
		return fmt.Errorf("git-cliff validation failed: %w", err)
	}
	return nil
}

// stepRunGoReleaser executes GoReleaser dry-run
func (o *DryRunOrchestrator) stepRunGoReleaser(ctx context.Context, cfg DryRunConfig) error {
	o.logStatus(ctx, cfg.CIOutput, "### 🏗️ Running GoReleaser Dry-Run")
	o.logger(ctx).Info("Running GoReleaser dry-run")
	if err := o.runGoReleaserDry(ctx); err != nil {
		return fmt.Errorf("GoReleaser dry-run failed: %w", err)
	}
	o.logger(ctx).Info("Completed GoReleaser dry-run")
	return nil
}

// stepExtractVersion extracts version from branch name
func (o *DryRunOrchestrator) stepExtractVersion(ctx context.Context, cfg DryRunConfig) (string, error) {
	o.logStatus(ctx, cfg.CIOutput, "### 📦 Validating NPM packages")
	o.logger(ctx).Info("Extracting version from branch")
	version, err := o.extractVersionFromBranch(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to extract version: %w", err)
	}
	o.logger(ctx).Info("Detected version", zap.String("version", version))
	return version, nil
}

// stepValidateNPM validates NPM package versions
// stepValidateNPM removed: tools/ update/validation is no longer part of the release process

// stepCommentPR creates PR comment with dry-run results
func (o *DryRunOrchestrator) stepCommentPR(ctx context.Context, _ DryRunConfig) error {
	o.logger(ctx).Info("Creating PR comment")
	if err := o.commentOnPR(ctx); err != nil {
		return fmt.Errorf("PR comment failed: %w", err)
	}
	o.logger(ctx).Info("PR comment created")
	return nil
}

// validateCliff runs git-cliff --unreleased --verbose
func (o *DryRunOrchestrator) validateCliff(ctx context.Context) error {
	log := o.logger(ctx)
	log.Info("Running git-cliff --unreleased --verbose")
	cmd := exec.CommandContext(ctx, "git-cliff", "--unreleased", "--verbose")
	// Find the repository root by walking up directories
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	repoRoot := findRepoRoot(wd)
	if repoRoot != "" {
		// Only run when inside an actual git repository
		if _, statErr := os.Stat(filepath.Join(repoRoot, ".git")); statErr == nil {
			cmd.Dir = repoRoot
			log.Info("Running git-cliff from repository root", zap.String("repo_root", repoRoot))
		} else {
			log.Info("Skipping git-cliff validation", zap.String("reason", "no .git directory"))
			return nil
		}
	} else {
		log.Info("Skipping git-cliff validation", zap.String("reason", "repository root not found"))
		return nil
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git-cliff failed: %w", err)
	}
	log.Info("git-cliff validation completed")
	return nil
}

// runGoReleaserDry runs goreleaser release --snapshot --skip=publish --clean
func (o *DryRunOrchestrator) runGoReleaserDry(ctx context.Context) error {
	return o.goreleaserSvc.Run(
		ctx,
		"release",
		"--snapshot",
		"--skip=publish",
		"--clean",
		"--release-notes="+ReleaseNotesOutputFile,
		"--release-header-tmpl="+releaseHeaderTmplPath,
		"--release-footer-tmpl="+releaseFooterTmplPath,
	)
}

// extractVersionFromBranch extracts version from GITHUB_HEAD_REF or branch name
func (o *DryRunOrchestrator) extractVersionFromBranch(ctx context.Context) (string, error) {
	headRef := os.Getenv(envGithubHeadRef)
	if headRef == "" {
		// Fallback to current branch
		branch, err := o.gitRepo.GetCurrentBranch(ctx)
		if err != nil {
			return "", err
		}
		headRef = branch
	}
	re := regexp.MustCompile(`v?\d+\.\d+\.\d+`)
	matches := re.FindStringSubmatch(headRef)
	if len(matches) == 0 {
		return "", fmt.Errorf("no version found in branch name: %s", headRef)
	}
	version := matches[0]
	version = strings.TrimPrefix(version, "v") // Remove 'v' prefix if present
	return version, nil
}

// validateNPMVersions runs UpdatePackageVersions (idempotent check; since branch may already have updates)
// validateNPMVersions removed

// commentOnPR reads metadata.json, builds body, adds comment via GithubRepo
func (o *DryRunOrchestrator) commentOnPR(ctx context.Context) error {
	prNumber := o.getPRNumber(ctx)
	if prNumber == 0 {
		o.logger(ctx).Info("Skipping PR comment", zap.String("reason", "no PR number found"))
		return nil
	}

	// Read metadata.json
	metadataPath := metadataJSONPath
	file, err := o.fsRepo.Open(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to open metadata.json: %w", err)
	}
	defer file.Close()
	var metadata map[string]any
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&metadata); err != nil {
		return fmt.Errorf("failed to parse metadata.json: %w", err)
	}

	// Build artifacts list (filter Archive types)
	artifactsList := "Not available."
	if arts, ok := metadata["artifacts"].([]any); ok {
		uniqueBuilds := make(map[string]struct{})
		for _, a := range arts {
			artMap, ok := a.(map[string]any)
			if !ok {
				continue
			}
			if artMap["type"] == artifactTypeArchive {
				goos, ok := artMap["goos"].(string)
				if !ok {
					continue
				}
				goarch, ok := artMap["goarch"].(string)
				if !ok {
					continue
				}
				uniqueBuilds[fmt.Sprintf("%s/%s", goos, goarch)] = struct{}{}
			}
		}
		var builds []string
		for b := range uniqueBuilds {
			builds = append(builds, fmt.Sprintf("- %s", b))
		}
		sort.Strings(builds)
		artifactsList = strings.Join(builds, "\n")
	}

	// Build comment body
	sha := os.Getenv(envGithubSHA)
	if len(sha) > 7 {
		sha = sha[:7]
	}
	body := fmt.Sprintf(`## ✅ Dry-Run Completed Successfully

### 📊 Build Summary
- **Version**: %s
- **Commit**: %s

### 📦 Built Artifacts
%s

---
*This is an automated comment from the release dry-run check.*
`, metadata["version"], sha, artifactsList)

	// Add comment
	return o.githubRepo.AddComment(ctx, prNumber, body)
}

// getPRNumber retrieves PR number from environment variables or GitHub event payload
func (o *DryRunOrchestrator) getPRNumber(_ context.Context) int {
	// Try environment variable first
	if prNumberStr := os.Getenv(envGithubIssueNumber); prNumberStr != "" {
		if prNumber, err := strconv.Atoi(prNumberStr); err == nil {
			return prNumber
		}
	}
	// Try GitHub event payload as fallback
	if eventPath := os.Getenv(envGithubEventPath); eventPath != "" {
		file, err := openGitHubEventPayload(eventPath)
		if err == nil {
			defer file.Close()
			var payload map[string]any
			if err := json.NewDecoder(file).Decode(&payload); err == nil {
				// Check for pull_request.number
				if pr, ok := payload["pull_request"].(map[string]any); ok {
					if n, ok := pr["number"].(float64); ok {
						return int(n)
					}
				}
				// Check for issue.number as fallback
				if issue, ok := payload["issue"].(map[string]any); ok {
					if n, ok := issue["number"].(float64); ok {
						return int(n)
					}
				}
			}
		}
	}
	return 0
}

func openGitHubEventPayload(path string) (*os.File, error) {
	cleanPath, ok := sanitizeGitHubEventPath(path)
	if !ok {
		return nil, fmt.Errorf("invalid github event path")
	}
	//nolint:gosec // Path is canonicalized and constrained to trusted GitHub Actions event locations.
	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		return nil, err
	}
	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("github event path is not a regular file")
	}
	//nolint:gosec // Path is canonicalized and constrained to trusted GitHub Actions event locations.
	return os.Open(cleanPath)
}

// logStatus records orchestrator status messages respecting CI output flags
func (o *DryRunOrchestrator) logStatus(ctx context.Context, ciOutput bool, message string) {
	if ciOutput {
		o.logger(ctx).Info("ci_status", zap.String("message", message))
		return
	}
	o.logger(ctx).Info(message)
}

// findRepoRoot walks up directories to find the git repository root
func findRepoRoot(startDir string) string {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func sanitizeGitHubEventPath(path string) (string, bool) {
	// Ensure the path is absolute
	if !filepath.IsAbs(path) {
		return "", false
	}

	// Clean the path to remove any traversal attempts
	cleanPath := filepath.Clean(path)
	if filepath.Base(cleanPath) != "event.json" {
		return "", false
	}

	// GitHub Actions typically sets this to a path like:
	// /home/runner/work/_temp/_github_workflow/event.json
	// or /github/workflow/event.json
	// We check that it contains expected patterns

	// Must end with .json
	if !strings.HasSuffix(cleanPath, ".json") {
		return "", false
	}

	// Should contain typical GitHub Actions path patterns
	validPatterns := []string{
		"/_temp/",
		"/workflow/",
		"/_github_workflow/",
		"/runner/",
	}

	hasValidPattern := false
	for _, pattern := range validPatterns {
		if strings.Contains(cleanPath, pattern) {
			hasValidPattern = true
			break
		}
	}

	if !hasValidPattern {
		return "", false
	}
	return cleanPath, true
}
