package repository

import (
	"context"
	"errors"
	"fmt"
)

var ErrGithubTokenRequired = errors.New("github token is required for GitHub operations")

type githubNoopRepository struct {
	owner string
	repo  string
}

func NewGithubNoopRepository(owner, repo string) GithubRepository {
	return &githubNoopRepository{owner: owner, repo: repo}
}

func NewGithubNoopExtendedRepository(owner, repo string) GithubExtendedRepository {
	return &githubNoopRepository{owner: owner, repo: repo}
}

func (r *githubNoopRepository) CreatePullRequest(_ context.Context, _, _, _, _ string) (int, error) {
	return 0, r.operationError("create pull request")
}

func (r *githubNoopRepository) CreateOrUpdatePR(
	_ context.Context,
	_, _, _, _ string,
	_ []string,
) error {
	return r.operationError("create or update pull request")
}

func (r *githubNoopRepository) AddComment(_ context.Context, _ int, _ string) error {
	return r.operationError("add comment")
}

func (r *githubNoopRepository) ClosePR(_ context.Context, _ int) error {
	return r.operationError("close pull request")
}

func (r *githubNoopRepository) GetPRStatus(_ context.Context, _ int) (string, error) {
	return "", r.operationError("query pull request status")
}

func (r *githubNoopRepository) operationError(action string) error {
	return fmt.Errorf("%w: unable to %s for %s/%s", ErrGithubTokenRequired, action, r.owner, r.repo)
}
