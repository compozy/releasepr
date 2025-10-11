package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/compozy/releasepr/internal/config"
	"github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
)

// githubRepository is the implementation of the GithubRepository interface.
type githubRepository struct {
	client *github.Client
	owner  string
	repo   string
}

// Note: GitHub token and owner/repo validation functions have been consolidated
// in the config package to avoid duplication and ensure consistency.

// NewGithubRepository creates a new GithubRepository with validation.
func NewGithubRepository(token, owner, repo string) (GithubRepository, error) {
	// Validate token format using the consolidated validator from config package
	if err := config.ValidateGitHubToken(token); err != nil {
		return nil, fmt.Errorf("invalid GitHub token: %w", err)
	}

	// Validate owner and repo names using the consolidated validator
	if err := config.ValidateGitHubOwnerRepo(owner, repo); err != nil {
		return nil, fmt.Errorf("invalid repository configuration: %w", err)
	}

	// Create OAuth2 client with the validated token
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: strings.TrimSpace(token)},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)

	// Create and return the repository
	ghRepo := &githubRepository{
		client: client,
		owner:  owner,
		repo:   repo,
	}

	return ghRepo, nil
}

// NewGithubExtendedRepository creates a new GithubExtendedRepository with validation.
func NewGithubExtendedRepository(token, owner, repo string) (GithubExtendedRepository, error) {
	// Validate token format using the consolidated validator from config package
	if err := config.ValidateGitHubToken(token); err != nil {
		return nil, fmt.Errorf("invalid GitHub token: %w", err)
	}

	// Validate owner and repo names using the consolidated validator
	if err := config.ValidateGitHubOwnerRepo(owner, repo); err != nil {
		return nil, fmt.Errorf("invalid repository configuration: %w", err)
	}

	// Create OAuth2 client with the validated token
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: strings.TrimSpace(token)},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)

	// Create and return the repository
	ghRepo := &githubRepository{
		client: client,
		owner:  owner,
		repo:   repo,
	}

	return ghRepo, nil
}

// CreatePullRequest creates a new pull request.
func (r *githubRepository) CreatePullRequest(ctx context.Context, title, body, head, base string) (int, error) {
	pr, _, err := r.client.PullRequests.Create(ctx, r.owner, r.repo, &github.NewPullRequest{
		Title: &title,
		Body:  &body,
		Head:  &head,
		Base:  &base,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create pull request: %w", err)
	}
	return pr.GetNumber(), nil
}

// CreateOrUpdatePR creates a new PR or updates an existing one.
func (r *githubRepository) CreateOrUpdatePR(
	ctx context.Context,
	head, base, title, body string,
	labels []string,
) error {
	fmt.Printf("CreateOrUpdatePR: owner=%s, repo=%s, head=%s, base=%s, title=%s\n",
		r.owner, r.repo, head, base, title)
	// First, try to find an existing PR
	fmt.Printf("Listing existing PRs for %s:%s -> %s\n", r.owner, head, base)
	prs, _, err := r.client.PullRequests.List(ctx, r.owner, r.repo, &github.PullRequestListOptions{
		Head:  fmt.Sprintf("%s:%s", r.owner, head),
		Base:  base,
		State: "open",
	})
	if err != nil {
		fmt.Printf("Failed to list pull requests: %v\n", err)
		return fmt.Errorf("failed to list pull requests: %w", err)
	}
	fmt.Printf("Found %d existing PRs\n", len(prs))
	if len(prs) > 0 {
		// Update existing PR
		pr := prs[0]
		fmt.Printf("Updating existing PR #%d\n", pr.GetNumber())
		_, _, err = r.client.PullRequests.Edit(ctx, r.owner, r.repo, pr.GetNumber(), &github.PullRequest{
			Title: &title,
			Body:  &body,
		})
		if err != nil {
			fmt.Printf("Failed to update PR #%d: %v\n", pr.GetNumber(), err)
			return fmt.Errorf("failed to update pull request: %w", err)
		}
		// Update labels
		if len(labels) > 0 {
			fmt.Printf("Adding labels to PR #%d: %v\n", pr.GetNumber(), labels)
			_, _, err = r.client.Issues.AddLabelsToIssue(ctx, r.owner, r.repo, pr.GetNumber(), labels)
			if err != nil {
				fmt.Printf("Failed to add labels: %v\n", err)
				return fmt.Errorf("failed to add labels: %w", err)
			}
		}
		fmt.Printf("Successfully updated PR #%d\n", pr.GetNumber())
		return nil
	}
	// Create new PR
	fmt.Printf("Creating new PR: %s -> %s\n", head, base)
	pr, _, err := r.client.PullRequests.Create(ctx, r.owner, r.repo, &github.NewPullRequest{
		Title: &title,
		Body:  &body,
		Head:  &head,
		Base:  &base,
	})
	if err != nil {
		fmt.Printf("Failed to create PR: %v\n", err)
		return fmt.Errorf("failed to create pull request: %w", err)
	}
	fmt.Printf("Created PR #%d\n", pr.GetNumber())
	// Add labels
	if len(labels) > 0 {
		fmt.Printf("Adding labels to new PR #%d: %v\n", pr.GetNumber(), labels)
		_, _, err = r.client.Issues.AddLabelsToIssue(ctx, r.owner, r.repo, pr.GetNumber(), labels)
		if err != nil {
			fmt.Printf("Failed to add labels: %v\n", err)
			return fmt.Errorf("failed to add labels: %w", err)
		}
	}
	fmt.Printf("Successfully created PR #%d\n", pr.GetNumber())
	return nil
}

// AddComment implementation
func (r *githubRepository) AddComment(ctx context.Context, prNumber int, body string) error {
	comment := &github.IssueComment{
		Body: github.Ptr(body),
	}
	_, _, err := r.client.Issues.CreateComment(ctx, r.owner, r.repo, prNumber, comment)
	if err != nil {
		return fmt.Errorf("failed to add comment to PR #%d: %w", prNumber, err)
	}
	return nil
}

// ClosePR closes a pull request
func (r *githubRepository) ClosePR(ctx context.Context, prNumber int) error {
	state := "closed"
	_, _, err := r.client.PullRequests.Edit(ctx, r.owner, r.repo, prNumber, &github.PullRequest{
		State: &state,
	})
	if err != nil {
		return fmt.Errorf("failed to close PR #%d: %w", prNumber, err)
	}
	return nil
}

// GetPRStatus returns the status of a pull request (open, closed, merged)
func (r *githubRepository) GetPRStatus(ctx context.Context, prNumber int) (string, error) {
	pr, _, err := r.client.PullRequests.Get(ctx, r.owner, r.repo, prNumber)
	if err != nil {
		return "", fmt.Errorf("failed to get PR #%d: %w", prNumber, err)
	}
	if pr.GetMerged() {
		return "merged", nil
	}
	return pr.GetState(), nil
}
