package orchestrator

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/compozy/releasepr/internal/logger"
	"github.com/compozy/releasepr/internal/repository"
	"go.uber.org/zap"
)

// CompensatingActions provides idempotent rollback operations for release workflow steps
type CompensatingActions struct {
	gitRepo    repository.GitExtendedRepository
	githubRepo repository.GithubExtendedRepository
}

// NewCompensatingActions creates a new compensating actions handler
func NewCompensatingActions(
	gitRepo repository.GitExtendedRepository,
	githubRepo repository.GithubExtendedRepository,
) *CompensatingActions {
	return &CompensatingActions{
		gitRepo:    gitRepo,
		githubRepo: githubRepo,
	}
}

func (ca *CompensatingActions) logger(ctx context.Context) *zap.Logger {
	return logger.FromContext(ctx).Named("orchestrator.compensating_actions")
}

// DeleteBranch idempotently deletes a branch locally and optionally from remote
func (ca *CompensatingActions) DeleteBranch(ctx context.Context, rollbackData map[string]any) error {
	log := ca.logger(ctx)
	branchName, ok := rollbackData["branch_name"].(string)
	if !ok {
		return fmt.Errorf("branch_name not found in rollback data")
	}
	// Check if branch was created in this session
	if !ca.wasCreatedInSession(rollbackData) {
		log.Info("Branch existed before session, skipping deletion", zap.String("branch", branchName))
		return nil
	}
	// Switch away from branch if currently on it
	if err := ca.switchFromBranchIfNeeded(ctx, branchName, rollbackData); err != nil {
		return err
	}
	// Delete local branch
	if err := ca.deleteLocalBranchIfExists(ctx, branchName); err != nil {
		return err
	}
	// Delete remote branch if pushed
	return ca.deleteRemoteBranchIfPushed(ctx, branchName, rollbackData)
}

// wasCreatedInSession checks if the branch was created in this session
func (ca *CompensatingActions) wasCreatedInSession(rollbackData map[string]any) bool {
	createdInSession, ok := rollbackData["created_in_session"].(bool)
	if !ok {
		return false // Default to false if not set
	}
	return createdInSession
}

// switchFromBranchIfNeeded switches away from the branch if currently on it
func (ca *CompensatingActions) switchFromBranchIfNeeded(
	ctx context.Context,
	branchName string,
	rollbackData map[string]any,
) error {
	currentBranch, err := ca.gitRepo.GetCurrentBranch(ctx)
	if err != nil || currentBranch != branchName {
		return nil // Not on the branch, nothing to do
	}
	// Try to switch to original branch
	if originalBranch, ok := rollbackData["original_branch"].(string); ok {
		if checkoutErr := ca.gitRepo.CheckoutBranch(ctx, originalBranch); checkoutErr == nil {
			return nil // Successfully switched
		}
	}
	// Try fallback branches
	if fallbackErr := ca.tryCheckoutFallbackBranch(ctx); fallbackErr != nil {
		return fmt.Errorf("cannot switch from branch %s: %w", branchName, fallbackErr)
	}
	return nil
}

// deleteLocalBranchIfExists deletes the local branch if it exists
func (ca *CompensatingActions) deleteLocalBranchIfExists(ctx context.Context, branchName string) error {
	if !ca.branchExistsLocally(ctx, branchName) {
		return nil
	}
	if err := ca.gitRepo.DeleteBranch(ctx, branchName); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("failed to delete local branch %s: %w", branchName, err)
		}
	}
	return nil
}

// deleteRemoteBranchIfPushed deletes the remote branch if it was pushed
func (ca *CompensatingActions) deleteRemoteBranchIfPushed(
	ctx context.Context,
	branchName string,
	rollbackData map[string]any,
) error {
	log := ca.logger(ctx)
	pushed, ok := rollbackData["pushed"].(bool)
	if !ok {
		pushed = false // Default to false if not set
	}
	if !pushed {
		return nil // Branch was never pushed, nothing to clean up
	}
	if !ca.branchExistsRemotely(ctx, branchName) {
		return nil // Branch doesn't exist remotely, already cleaned up
	}
	// Attempt to delete remote branch with retry logic
	const maxRetries = 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := ca.gitRepo.DeleteRemoteBranch(ctx, branchName); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return nil // Branch was deleted between checks, success
			}
			if attempt == maxRetries {
				return fmt.Errorf(
					"failed to delete remote branch %s after %d attempts: %w",
					branchName,
					maxRetries,
					err,
				)
			}
			log.Warn("Failed to delete remote branch, retrying",
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", maxRetries),
				zap.String("branch", branchName),
				zap.Error(err),
			)
			continue
		}
		// Success
		log.Info("Deleted remote branch", zap.String("branch", branchName))
		return nil
	}
	return nil
}

// RestoreFiles idempotently restores modified files to their original state
func (ca *CompensatingActions) RestoreFiles(ctx context.Context, rollbackData map[string]any) error {
	log := ca.logger(ctx)
	modifiedFiles, ok := rollbackData["modified_files"].([]string)
	if !ok {
		// Try interface{} slice (JSON unmarshaling)
		if filesInterface, ok := rollbackData["modified_files"].([]any); ok {
			modifiedFiles = make([]string, len(filesInterface))
			for i, f := range filesInterface {
				if str, ok := f.(string); ok {
					modifiedFiles[i] = str
				}
			}
		} else {
			return nil // No files to restore
		}
	}
	for _, file := range modifiedFiles {
		// Check if file has uncommitted changes
		if ca.fileHasChanges(ctx, file) {
			if err := ca.gitRepo.RestoreFile(ctx, file); err != nil {
				if !os.IsNotExist(err) {
					log.Warn("Failed to restore file", zap.String("file", file), zap.Error(err))
				}
			}
		}
	}
	return nil
}

// ResetCommit idempotently undoes a commit
func (ca *CompensatingActions) ResetCommit(ctx context.Context, rollbackData map[string]any) error {
	log := ca.logger(ctx)
	commitSHA, ok := rollbackData["commit_sha"].(string)
	if !ok || commitSHA == "" || commitSHA == "HEAD" {
		// No specific commit to reset
		return nil
	}
	// Check if the commit is still the HEAD
	currentHead, err := ca.gitRepo.GetHeadCommit(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current HEAD: %w", err)
	}
	// If we're not at the commit anymore, it's already been reset
	if !strings.HasPrefix(currentHead, commitSHA) {
		log.Info("Commit already reset", zap.String("commit", commitSHA))
		return nil
	}
	// Reset to the commit before this one
	if err := ca.gitRepo.ResetHard(ctx, commitSHA+"~1"); err != nil {
		// If the commit doesn't exist or we're at the first commit, ignore
		if strings.Contains(err.Error(), "unknown revision") ||
			strings.Contains(err.Error(), "does not have a parent") {
			log.Warn("Cannot reset commit", zap.String("commit", commitSHA), zap.Error(err))
			return nil
		}
		return fmt.Errorf("failed to reset commit %s: %w", commitSHA, err)
	}
	return nil
}

// ClosePullRequest idempotently closes a pull request with a rollback comment
func (ca *CompensatingActions) ClosePullRequest(ctx context.Context, rollbackData map[string]any) error {
	log := ca.logger(ctx)
	prNumber := ca.extractPRNumber(rollbackData)
	if prNumber == 0 {
		// No PR to close
		return nil
	}
	// Check if PR is already closed
	prStatus, err := ca.githubRepo.GetPRStatus(ctx, prNumber)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// PR doesn't exist, nothing to do
			return nil
		}
		return fmt.Errorf("failed to check PR status: %w", err)
	}
	if prStatus == "closed" {
		log.Info("Pull request already closed", zap.Int("pr_number", prNumber))
		return nil
	}
	// Add a comment explaining the rollback
	comment := "🔄 This pull request was automatically closed due to a rollback of the release workflow."
	if err := ca.githubRepo.AddComment(ctx, prNumber, comment); err != nil {
		log.Warn("Failed to add rollback comment", zap.Int("pr_number", prNumber), zap.Error(err))
	}
	// Close the PR
	if err := ca.githubRepo.ClosePR(ctx, prNumber); err != nil {
		if !strings.Contains(err.Error(), "already closed") {
			return fmt.Errorf("failed to close PR #%d: %w", prNumber, err)
		}
	}
	return nil
}

// NoOp is a no-operation compensating action for operations that don't need rollback
func (ca *CompensatingActions) NoOp(_ context.Context, _ map[string]any) error {
	return nil
}

// Helper methods for idempotency checks

func (ca *CompensatingActions) branchExistsLocally(ctx context.Context, branchName string) bool {
	branches, err := ca.gitRepo.ListLocalBranches(ctx)
	if err != nil {
		return false
	}
	return slices.Contains(branches, branchName)
}

func (ca *CompensatingActions) branchExistsRemotely(ctx context.Context, branchName string) bool {
	branches, err := ca.gitRepo.ListRemoteBranches(ctx)
	if err != nil {
		return false
	}
	for _, branch := range branches {
		if strings.HasSuffix(branch, "/"+branchName) {
			return true
		}
	}
	return false
}

func (ca *CompensatingActions) fileHasChanges(ctx context.Context, file string) bool {
	status, err := ca.gitRepo.GetFileStatus(ctx, file)
	if err != nil {
		return false
	}
	return status != "clean"
}

func (ca *CompensatingActions) tryCheckoutFallbackBranch(ctx context.Context) error {
	// Try main first, then master
	if err := ca.gitRepo.CheckoutBranch(ctx, "main"); err == nil {
		return nil
	}
	if err := ca.gitRepo.CheckoutBranch(ctx, "master"); err == nil {
		return nil
	}
	return fmt.Errorf("failed to checkout fallback branch (tried main and master)")
}

func (ca *CompensatingActions) extractPRNumber(rollbackData map[string]any) int {
	if prNumber, ok := rollbackData["pr_number"].(int); ok {
		return prNumber
	}
	// Try float64 (JSON unmarshaling)
	if prFloat, ok := rollbackData["pr_number"].(float64); ok {
		return int(prFloat)
	}
	return 0
}
