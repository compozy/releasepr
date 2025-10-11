package usecase

import (
	"context"
	"fmt"

	"github.com/compozy/releasepr/internal/logger"
	"github.com/compozy/releasepr/internal/repository"
	"go.uber.org/zap"
)

// CreateReleaseBranchUseCase contains the logic for the create-release-branch command.

type CreateReleaseBranchUseCase struct {
	GitRepo repository.GitRepository
}

// Execute runs the use case.
func (uc *CreateReleaseBranchUseCase) Execute(ctx context.Context, branchName string) error {
	log := logger.FromContext(ctx)
	log.Info("Creating local branch", zap.String("branch", branchName))
	if err := uc.GitRepo.CreateBranch(ctx, branchName); err != nil {
		return fmt.Errorf("failed to create release branch: %w", err)
	}
	log.Info("Local branch created successfully", zap.String("branch", branchName))
	return nil
}
