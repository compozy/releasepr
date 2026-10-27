package repository

import "context"

// GithubExtendedRepository extends GithubRepository with additional operations for orchestration.
type GithubExtendedRepository interface {
	GithubRepository
	ListOpenPRs(ctx context.Context, base string) ([]PullRequest, error)
	// CreateOrUpdatePR creates a new PR or updates an existing one
	CreateOrUpdatePR(ctx context.Context, head, base, title, body string, labels []string) error
	// AddComment adds a comment to a PR/issue
	AddComment(ctx context.Context, prNumber int, body string) error
	// ClosePR closes a pull request
	ClosePR(ctx context.Context, prNumber int) error
	// GetPRStatus returns the status of a pull request (open, closed, merged)
	GetPRStatus(ctx context.Context, prNumber int) (string, error)
}

// PullRequest contains the facts needed to identify an owned release PR.
type PullRequest struct {
	Number         int
	URL            string
	Title          string
	Head           string
	SameRepository bool
	Labels         []string
}
