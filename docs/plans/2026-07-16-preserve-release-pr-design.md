# Preserve Existing Release Pull Requests

## Problem

Every CI run uses `pr-release --force --enable-rollback`. When the release branch already exists remotely, the force path deletes that remote branch. GitHub closes the open pull request attached to the deleted branch, and the later create-or-update operation cannot find it because it only searches open pull requests. The run then creates a duplicate pull request for the same version.

## Design

Forced release refreshes must preserve a preexisting remote release branch. The workflow may delete and recreate a stale local release branch from the current base branch, but it must retain the remote-existence state. The push step will then use the existing force-push path, which updates the remote branch without changing the pull request identity.

The GitHub repository behavior remains unchanged: it updates an open pull request matching the release head and base branches. If a pull request was manually closed, it remains closed; a later run may create a new pull request instead of reopening it.

Rollback metadata must continue to distinguish local and remote branch ownership. A local branch recreated during the run may be removed during rollback, while a remote branch that existed before the run must never be deleted by compensation.

## Testing

The branch lifecycle belongs to the orchestrator layer and its canonical `TestPRReleaseOrchestrator_Execute` suite. The regression invariant is: when a forced run observes an existing remote release branch, it does not call `DeleteRemoteBranch`, force-pushes the refreshed local branch, and proceeds through `CreateOrUpdatePR`.
