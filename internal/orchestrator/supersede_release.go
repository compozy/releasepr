package orchestrator

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/compozy/releasepr/internal/repository"
	"go.uber.org/zap"
)

var releasePRNumberSuffix = regexp.MustCompile(` \(#\d+\)$`)
var releaseMergeSubject = regexp.MustCompile(`^Merge pull request #\d+ from [^/]+/release/(v\S+)$`)

func (o *PRReleaseOrchestrator) releaseAlreadyMerged(ctx context.Context, version string) (bool, error) {
	message, err := o.gitRepo.BaseHeadMessage(ctx, "main")
	if err != nil {
		return false, fmt.Errorf("failed to inspect base HEAD: %w", err)
	}
	if !releaseHeadMatches(message, version) {
		return false, nil
	}
	exists, err := o.gitRepo.TagExists(ctx, version)
	if err != nil {
		return false, fmt.Errorf("failed to inspect release tag: %w", err)
	}
	if exists {
		return false, nil
	}
	o.logger(ctx).Info("No-op: release already merged at base HEAD; waiting for tag", zap.String("version", version))
	return true, nil
}

func ownedReleasePR(pr *repository.PullRequest) bool {
	if !pr.SameRepository {
		return false
	}
	version, ok := strings.CutPrefix(pr.Head, "release/v")
	if !ok {
		return false
	}
	if _, err := semver.StrictNewVersion(version); err != nil {
		return false
	}
	return slices.Contains(pr.Labels, releasePRLabelPending) || pr.Title == releasePullRequestTitle("v"+version)
}

func (o *PRReleaseOrchestrator) supersedeReleasePRs(ctx context.Context, branch string, dryRun bool) error {
	prs, err := o.githubRepo.ListOpenPRs(ctx, "main")
	if err != nil {
		return fmt.Errorf("failed to inspect superseded release PRs: %w", err)
	}
	var replacement repository.PullRequest
	var stale []repository.PullRequest
	for _, pr := range prs {
		if !ownedReleasePR(&pr) {
			continue
		}
		if pr.Head == branch {
			replacement = pr
		} else {
			stale = append(stale, pr)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	if !dryRun && (replacement.Number == 0 || replacement.URL == "") {
		return fmt.Errorf("cannot supersede release PRs: replacement PR for %s was not found", branch)
	}
	for _, pr := range stale {
		if dryRun {
			o.logger(ctx).Info("Dry-run: would close superseded release PR",
				zap.Int("pr_number", pr.Number), zap.String("superseding_branch", branch))
			continue
		}
		if err := o.closeSupersededPR(ctx, pr.Number, replacement.URL); err != nil {
			return err
		}
	}
	return nil
}

func (o *PRReleaseOrchestrator) closeSupersededPR(ctx context.Context, number int, replacementURL string) error {
	if err := o.githubRepo.AddComment(ctx, number, "Superseded by "+replacementURL+"."); err != nil {
		return fmt.Errorf("failed to comment on superseded PR #%d: %w", number, err)
	}
	if err := o.githubRepo.ClosePR(ctx, number); err != nil {
		return fmt.Errorf("failed to close superseded PR #%d: %w", number, err)
	}
	o.logger(ctx).
		Info("Closed superseded release PR", zap.Int("pr_number", number), zap.String("superseding_pr", replacementURL))
	return nil
}

func releaseHeadMatches(message, version string) bool {
	subject, body, _ := strings.Cut(message, "\n")
	subject = releasePRNumberSuffix.ReplaceAllString(subject, "")
	if subject == releasePullRequestTitle(version) || subject == "build: prepare release "+version {
		return true
	}
	match := releaseMergeSubject.FindStringSubmatch(subject)
	title, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	return len(match) == 2 && match[1] == version && title == releasePullRequestTitle(version)
}
