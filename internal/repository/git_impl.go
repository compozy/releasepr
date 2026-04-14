package repository

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// gitRepository is the implementation of the GitRepository interface.

type gitRepository struct {
	repo               *git.Repository
	pushTimeoutMinutes int
}

// NewGitRepository creates a new GitRepository.
func NewGitRepository() (GitRepository, error) {
	repo, err := git.PlainOpen(".")
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}
	return &gitRepository{repo: repo, pushTimeoutMinutes: 2}, nil
}

// NewGitExtendedRepository creates a new GitExtendedRepository with all extended operations.
func NewGitExtendedRepository() (GitExtendedRepository, error) {
	repo, err := git.PlainOpen(".")
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}
	return &gitRepository{repo: repo, pushTimeoutMinutes: 2}, nil
}

// NewGitExtendedRepositoryWithTimeout creates a new GitExtendedRepository with custom timeout.
func NewGitExtendedRepositoryWithTimeout(timeoutMinutes int) (GitExtendedRepository, error) {
	repo, err := git.PlainOpen(".")
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}
	if timeoutMinutes < 1 {
		timeoutMinutes = 2
	}
	return &gitRepository{repo: repo, pushTimeoutMinutes: timeoutMinutes}, nil
}

// LatestTag returns the latest git tag.
func (r *gitRepository) LatestTag(ctx context.Context) (string, error) {
	// First, try to fetch tags from remote to ensure we have the latest
	remote, err := r.repo.Remote("origin")
	if err == nil {
		// Fetch tags from remote with timeout (ignore error if already up to date)
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		//nolint:errcheck // We intentionally ignore the error as local tags are sufficient
		_ = remote.FetchContext(fetchCtx, &git.FetchOptions{
			RefSpecs: []config.RefSpec{
				config.RefSpec("+refs/tags/*:refs/tags/*"),
			},
			Auth: r.getAuth(),
		})
	}
	tagRefs, err := r.repo.Tags()
	if err != nil {
		return "", fmt.Errorf("failed to get tags: %w", err)
	}
	var latestTag string
	var latestCommitTime time.Time
	if err := tagRefs.ForEach(func(ref *plumbing.Reference) error {
		// Try to get the commit directly first (lightweight tag)
		commit, err := r.repo.CommitObject(ref.Hash())
		if err != nil {
			// If that fails, try to get it as an annotated tag
			tag, err := r.repo.TagObject(ref.Hash())
			if err != nil {
				return nil // Skip this tag if we can't resolve it
			}
			commit, err = r.repo.CommitObject(tag.Target)
			if err != nil {
				return nil // Skip if we can't get the commit
			}
		}
		if commit.Committer.When.After(latestCommitTime) {
			latestCommitTime = commit.Committer.When
			latestTag = ref.Name().Short()
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to iterate tags: %w", err)
	}
	return latestTag, nil
}

// fetchTagIfNeeded fetches a tag from remote if it doesn't exist locally.
func (r *gitRepository) fetchTagIfNeeded(ctx context.Context, tag string) (*plumbing.Reference, error) {
	tagRef, err := r.repo.Tag(tag)
	if err == nil {
		return tagRef, nil
	}
	// Tag doesn't exist locally, try to fetch it from remote
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return nil, fmt.Errorf("failed to get remote: %w", err)
	}
	// Fetch tags from remote with timeout
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := remote.FetchContext(fetchCtx, &git.FetchOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/tags/*:refs/tags/*"),
		},
		Auth: r.getAuth(),
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("failed to fetch tags from remote: %w", err)
	}
	// Try to get the tag again
	tagRef, err = r.repo.Tag(tag)
	if err != nil {
		return nil, fmt.Errorf("failed to get tag %s after fetching: %w", tag, err)
	}
	return tagRef, nil
}

// resolveTagCommit resolves a tag reference to its commit hash.
func (r *gitRepository) resolveTagCommit(tagRef *plumbing.Reference) (plumbing.Hash, error) {
	// Try as lightweight tag first
	if commit, err := r.repo.CommitObject(tagRef.Hash()); err == nil {
		return commit.Hash, nil
	}
	// Try as annotated tag
	if tagObj, err := r.repo.TagObject(tagRef.Hash()); err == nil {
		if commit, err := r.repo.CommitObject(tagObj.Target); err == nil {
			return commit.Hash, nil
		}
	}
	return plumbing.Hash{}, fmt.Errorf("failed to resolve commit for tag")
}

// countCommitsSince counts commits from HEAD to the given commit hash.
func (r *gitRepository) countCommitsSince(tagCommitHash plumbing.Hash) (int, error) {
	head, err := r.repo.Head()
	if err != nil {
		return 0, fmt.Errorf("failed to get HEAD: %w", err)
	}
	headCommit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		return 0, fmt.Errorf("failed to get HEAD commit: %w", err)
	}
	commits, err := r.repo.Log(&git.LogOptions{From: headCommit.Hash})
	if err != nil {
		return 0, fmt.Errorf("failed to get commits: %w", err)
	}
	var count int
	err = commits.ForEach(func(c *object.Commit) error {
		if c.Hash == tagCommitHash {
			return storer.ErrStop
		}
		count++
		return nil
	})
	if err != nil && err != storer.ErrStop {
		return 0, fmt.Errorf("failed to iterate commits: %w", err)
	}
	return count, nil
}

// CommitsSinceTag returns the number of commits since the given tag.
func (r *gitRepository) CommitsSinceTag(ctx context.Context, tag string) (int, error) {
	tagRef, err := r.fetchTagIfNeeded(ctx, tag)
	if err != nil {
		return 0, err
	}
	tagCommitHash, err := r.resolveTagCommit(tagRef)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve tag %s: %w", tag, err)
	}
	return r.countCommitsSince(tagCommitHash)
}

// TagExists checks if a tag exists.
func (r *gitRepository) TagExists(_ context.Context, tag string) (bool, error) {
	_, err := r.repo.Tag(tag)
	if err == git.ErrTagNotFound {
		return false, nil
	}
	if err != nil && err != git.ErrTagNotFound {
		return false, fmt.Errorf("failed to check tag %s: %w", tag, err)
	}
	return err == nil, nil
}

// CreateBranch creates a new branch.
func (r *gitRepository) CreateBranch(_ context.Context, name string) error {
	// Check if branch already exists
	branchRef := plumbing.NewBranchReferenceName(name)
	_, err := r.repo.Reference(branchRef, false)
	if err == nil {
		return fmt.Errorf("branch %s already exists", name)
	}

	head, err := r.repo.Head()
	if err != nil {
		return err
	}

	ref := plumbing.NewHashReference(branchRef, head.Hash())
	return r.repo.Storer.SetReference(ref)
}

// CreateTag creates a new tag.
func (r *gitRepository) CreateTag(_ context.Context, tag, msg string) error {
	head, err := r.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	_, err = r.repo.CreateTag(tag, head.Hash(), &git.CreateTagOptions{
		Message: msg,
		Tagger: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create tag %s: %w", tag, err)
	}
	return nil
}

// getAuth returns authentication configuration for GitHub Actions
func (r *gitRepository) getAuth() *http.BasicAuth {
	// Check for GITHUB_TOKEN environment variable (used in GitHub Actions)
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		// Also check for COMPOZY_RELEASE_GITHUB_TOKEN
		token = os.Getenv("COMPOZY_RELEASE_GITHUB_TOKEN")
	}
	if token == "" {
		return nil
	}
	// Use x-access-token as username for GitHub token authentication
	return &http.BasicAuth{
		Username: "x-access-token",
		Password: token,
	}
}

// getGitEnv returns environment variables for native git CLI authentication
func (r *gitRepository) getGitEnv() []string {
	// Disable terminal prompts to prevent hanging on auth failures
	return []string{"GIT_TERMINAL_PROMPT=0"}
}

// getWorkingDirectory returns the git repository working directory
func (r *gitRepository) getWorkingDirectory() string {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return "."
	}
	return worktree.Filesystem.Root()
}

// getAuthenticatedURL constructs a git remote URL with embedded credentials.
// Returns the authenticated URL, the auth object (for sanitization), and any error.
func (r *gitRepository) getAuthenticatedURL() (string, *http.BasicAuth, error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", nil, fmt.Errorf("failed to get remote 'origin': %w", err)
	}
	if len(remote.Config().URLs) == 0 {
		return "", nil, fmt.Errorf("no URL found for remote 'origin'")
	}
	rawURL := remote.Config().URLs[0]
	auth := r.getAuth()
	if auth == nil {
		return rawURL, nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse remote URL %q: %w", rawURL, err)
	}
	u.User = url.UserPassword(auth.Username, auth.Password)
	return u.String(), auth, nil
}

// sanitizeOutput removes sensitive information from git command output
func sanitizeOutput(output string, authURL string, auth *http.BasicAuth) string {
	sanitized := output
	if auth != nil && auth.Password != "" {
		sanitized = strings.ReplaceAll(sanitized, authURL, "[REDACTED_URL]")
		sanitized = strings.ReplaceAll(sanitized, auth.Password, "[REDACTED_TOKEN]")
	}
	return sanitized
}

// PushTag pushes a tag to the remote.
func (r *gitRepository) PushTag(ctx context.Context, tag string) error {
	pushCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return r.repo.PushContext(pushCtx, &git.PushOptions{
		RefSpecs: []config.RefSpec{config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", tag, tag))},
		Auth:     r.getAuth(),
	})
}

// PushBranch pushes a branch to the remote using native git CLI for reliable timeout enforcement.
// NOTE: Using native git instead of go-git because go-git's PushContext doesn't respect context
// cancellation during network I/O, causing operations to hang for 10+ minutes despite timeouts.
func (r *gitRepository) PushBranch(ctx context.Context, name string) error {
	timeout := time.Duration(r.pushTimeoutMinutes) * time.Minute
	pushCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	authURL, auth, err := r.getAuthenticatedURL()
	if err != nil {
		return fmt.Errorf("failed to prepare authenticated URL for push: %w", err)
	}
	refSpec := fmt.Sprintf("refs/heads/%s:refs/heads/%s", name, name)
	cmd := exec.CommandContext(pushCtx, "git", "push", authURL, refSpec)
	cmd.Dir = r.getWorkingDirectory()
	cmd.Env = append(os.Environ(), r.getGitEnv()...)
	if output, err := cmd.CombinedOutput(); err != nil {
		sanitizedOutput := sanitizeOutput(string(output), authURL, auth)
		return fmt.Errorf("failed to push branch %s: %w (output: %s)", name, err, sanitizedOutput)
	}
	return nil
}

// PushBranchForce pushes a branch to the remote with force using native git CLI.
// NOTE: Using native git instead of go-git for reliable timeout enforcement (see PushBranch).
func (r *gitRepository) PushBranchForce(ctx context.Context, name string) error {
	timeout := time.Duration(r.pushTimeoutMinutes) * time.Minute
	pushCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	authURL, auth, err := r.getAuthenticatedURL()
	if err != nil {
		return fmt.Errorf("failed to prepare authenticated URL for push: %w", err)
	}
	refSpec := fmt.Sprintf("refs/heads/%s:refs/heads/%s", name, name)
	cmd := exec.CommandContext(pushCtx, "git", "push", "--force", authURL, refSpec)
	cmd.Dir = r.getWorkingDirectory()
	cmd.Env = append(os.Environ(), r.getGitEnv()...)
	if output, err := cmd.CombinedOutput(); err != nil {
		sanitizedOutput := sanitizeOutput(string(output), authURL, auth)
		return fmt.Errorf("failed to force push branch %s: %w (output: %s)", name, err, sanitizedOutput)
	}
	return nil
}

// CheckoutBranch switches to the specified branch using native git for performance.
func (r *gitRepository) CheckoutBranch(ctx context.Context, name string) error {
	checkoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(checkoutCtx, "git", "checkout", name)
	cmd.Dir = r.getWorkingDirectory()
	cmd.Env = append(os.Environ(), r.getGitEnv()...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w (output: %s)", name, err, string(output))
	}
	return nil
}

// ConfigureUser sets the git user configuration.
func (r *gitRepository) ConfigureUser(_ context.Context, name, email string) error {
	cfg, err := r.repo.Config()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	cfg.User.Name = name
	cfg.User.Email = email
	return r.repo.Storer.SetConfig(cfg)
}

// AddFiles stages files matching the pattern.
func (r *gitRepository) AddFiles(_ context.Context, pattern string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}
	// AddGlob can return an error if no files match the pattern
	// or if there are no changes to stage. We should not fail in these cases.
	err = w.AddGlob(pattern)
	if err != nil && err.Error() != "glob pattern did not match any files" {
		return fmt.Errorf("failed to add files with pattern %s: %w", pattern, err)
	}
	return nil
}

// Commit creates a commit with the given message.
func (r *gitRepository) Commit(_ context.Context, message string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}
	_, err = w.Commit(message, &git.CommitOptions{})
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}
	return nil
}

// GetCurrentBranch returns the name of the current branch.
func (r *gitRepository) GetCurrentBranch(_ context.Context) (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	return head.Name().Short(), nil
}

// DeleteBranch deletes a local branch.
func (r *gitRepository) DeleteBranch(_ context.Context, name string) error {
	err := r.repo.Storer.RemoveReference(plumbing.NewBranchReferenceName(name))
	if err != nil {
		return fmt.Errorf("failed to delete branch %s: %w", name, err)
	}
	return nil
}

// DeleteRemoteBranch deletes a remote branch.
func (r *gitRepository) DeleteRemoteBranch(ctx context.Context, name string) error {
	deleteCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	refSpec := config.RefSpec(":refs/heads/" + name)
	err := r.repo.PushContext(deleteCtx, &git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
		Auth:       r.getAuth(),
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to delete remote branch %s: %w", name, err)
	}
	return nil
}

// MoveFile moves a tracked file using native git so rename state is preserved.
func (r *gitRepository) MoveFile(ctx context.Context, from, to string) error {
	moveCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(moveCtx, "git", "mv", from, to)
	cmd.Dir = r.getWorkingDirectory()
	cmd.Env = append(os.Environ(), r.getGitEnv()...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to move file from %s to %s: %w (output: %s)", from, to, err, string(output))
	}
	return nil
}

// RestoreFile restores a file to its state in HEAD.
func (r *gitRepository) RestoreFile(ctx context.Context, path string) error {
	restoreCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(restoreCtx, "git", "checkout", "--", path)
	cmd.Dir = r.getWorkingDirectory()
	cmd.Env = append(os.Environ(), r.getGitEnv()...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restore file %s: %w (output: %s)", path, err, string(output))
	}
	return nil
}

// ResetHard performs a hard reset to the specified reference.
func (r *gitRepository) ResetHard(ctx context.Context, ref string) error {
	resetCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(resetCtx, "git", "reset", "--hard", ref)
	cmd.Dir = r.getWorkingDirectory()
	cmd.Env = append(os.Environ(), r.getGitEnv()...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reset to %s: %w (output: %s)", ref, err, string(output))
	}
	return nil
}

// GetHeadCommit returns the SHA of the current HEAD commit.
func (r *gitRepository) GetHeadCommit(_ context.Context) (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	return head.Hash().String(), nil
}

// ListLocalBranches returns a list of all local branch names.
func (r *gitRepository) ListLocalBranches(_ context.Context) ([]string, error) {
	iter, err := r.repo.Branches()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	var branches []string
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, ref.Name().Short())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate branches: %w", err)
	}
	return branches, nil
}

// ListRemoteBranches returns a list of all remote branch names.
func (r *gitRepository) ListRemoteBranches(ctx context.Context) ([]string, error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return nil, fmt.Errorf("failed to get remote: %w", err)
	}
	// Use context with timeout to prevent hanging
	refs, err := remote.ListContext(ctx, &git.ListOptions{
		Auth: r.getAuth(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list remote refs: %w", err)
	}
	var branches []string
	for _, ref := range refs {
		if ref.Name().IsBranch() {
			// Returns in format "origin/branch-name"
			branches = append(branches, "origin/"+ref.Name().Short())
		}
	}
	return branches, nil
}

// RemoteBranchExists checks if a specific branch exists on the remote.
// This is more efficient than ListRemoteBranches when checking a single branch.
func (r *gitRepository) RemoteBranchExists(ctx context.Context, branchName string) (bool, error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return false, fmt.Errorf("failed to get remote: %w", err)
	}
	refs, err := remote.ListContext(ctx, &git.ListOptions{
		Auth: r.getAuth(),
	})
	if err != nil {
		return false, fmt.Errorf("failed to list remote refs: %w", err)
	}
	targetRef := plumbing.NewBranchReferenceName(branchName)
	for _, ref := range refs {
		if ref.Name() == targetRef {
			return true, nil
		}
	}
	return false, nil
}

// GetFileStatus returns the git status of a specific file.
// Returns "clean" if the file has no changes, "modified" if it has uncommitted changes.
func (r *gitRepository) GetFileStatus(_ context.Context, path string) (string, error) {
	w, err := r.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}
	status, err := w.Status()
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}
	fileStatus := status.File(path)
	if fileStatus.Worktree == git.Unmodified && fileStatus.Staging == git.Unmodified {
		return "clean", nil
	}
	return "modified", nil
}
