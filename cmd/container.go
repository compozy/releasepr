package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/compozy/releasepr/internal/config"
	"github.com/compozy/releasepr/internal/logger"
	"github.com/compozy/releasepr/internal/orchestrator"
	"github.com/compozy/releasepr/internal/repository"
	"github.com/compozy/releasepr/internal/service"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// container holds all the dependencies for the application.

type container struct {
	cfg *config.Config

	fsRepo   repository.FileSystemRepository
	gitRepo  repository.GitRepository
	ghRepo   repository.GithubRepository
	cliffSvc service.CliffService
	npmSvc   service.NpmService
}

// newContainer creates a new container with all the dependencies.
func newContainer() (*container, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	fsRepo := repository.FileSystemRepository(afero.NewOsFs())
	gitRepo, err := repository.NewGitRepository()
	if err != nil {
		return nil, err
	}

	var ghRepo repository.GithubRepository
	if cfg.GithubToken != "" {
		ghRepo, err = repository.NewGithubRepository(cfg.GithubToken, cfg.GithubOwner, cfg.GithubRepo)
		if err != nil {
			return nil, err
		}
	} else {
		ghRepo = repository.NewGithubNoopRepository(cfg.GithubOwner, cfg.GithubRepo)
	}

	cliffSvc := service.NewCliffService()
	npmSvc := service.NewNpmService()

	return &container{
		cfg:      cfg,
		fsRepo:   fsRepo,
		gitRepo:  gitRepo,
		ghRepo:   ghRepo,
		cliffSvc: cliffSvc,
		npmSvc:   npmSvc,
	}, nil
}

// InitCommands initializes all commands with their dependencies
func InitCommands() error {
	c, err := newContainer()
	if err != nil {
		return err
	}
	ctx := context.Background()
	ctx = config.IntoContext(ctx, c.cfg)
	appLogger, err := logger.New(c.cfg.LoggerConfig())
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	ctx = logger.IntoContext(ctx, appLogger)
	rootCmd.SetContext(ctx)
	rootCmd.PersistentPostRunE = func(cmd *cobra.Command, _ []string) error {
		return logger.Sync(logger.FromContext(cmd.Context()))
	}

	// Individual commands have been replaced by orchestrator commands

	// Add orchestrator-based commands
	if err := addOrchestratorCommands(ctx, c); err != nil {
		return err
	}

	rootCmd.AddCommand(newVersionCmd())

	return nil
}

// addOrchestratorCommands adds the new consolidated commands
func addOrchestratorCommands(ctx context.Context, c *container) error {
	log := logger.FromContext(ctx).Named("cmd.container")
	// Initialize extended repositories for orchestrators
	gitExtRepo, err := repository.NewGitExtendedRepositoryWithTimeout(c.cfg.GitPushTimeoutMinutes)
	if err != nil {
		return fmt.Errorf("failed to initialize git extended repository: %w", err)
	}

	token := c.cfg.GithubToken
	tokenSource := "config"
	if token == "" {
		// #nosec G101 -- false positive: this is reading env var, not a credential
		token = os.Getenv("RELEASE_TOKEN")
		if token != "" {
			// #nosec G101 -- false positive: this is a diagnostic message, not a credential
			tokenSource = "RELEASE_TOKEN env var"
		}
	}
	owner := c.cfg.GithubOwner
	repo := c.cfg.GithubRepo
	log.Info("GitHub configuration",
		zap.String("owner", owner),
		zap.String("repo", repo),
		zap.String("token_source", tokenSource),
		zap.Bool("token_present", token != ""),
		zap.Int("token_length", len(token)),
	)
	if owner == "" || repo == "" {
		return fmt.Errorf("github owner/repo not configured; set GITHUB_REPOSITORY or config values")
	}
	var githubExtRepo repository.GithubExtendedRepository
	if token == "" {
		log.Warn("GitHub token not provided; GitHub operations will be skipped")
		githubExtRepo = repository.NewGithubNoopExtendedRepository(owner, repo)
	} else {
		log.Info("Initializing GitHub extended repository", zap.Int("token_length", len(token)))
		var err error
		githubExtRepo, err = repository.NewGithubExtendedRepository(token, owner, repo)
		if err != nil {
			return fmt.Errorf("failed to initialize GitHub extended repository: %w", err)
		}
		log.Info("Initialized GitHub extended repository", zap.String("owner", owner), zap.String("repo", repo))
	}

	// Create PR Release orchestrator
	prOrch := orchestrator.NewPRReleaseOrchestrator(
		gitExtRepo,
		githubExtRepo,
		c.fsRepo,
		c.cliffSvc,
		c.npmSvc,
	)
	rootCmd.AddCommand(NewPRReleaseCmd(prOrch))

	// Create Dry Run orchestrator
	goreleaserSvc := service.NewGoReleaserService()
	dryRunOrch := orchestrator.NewDryRunOrchestrator(
		gitExtRepo,
		githubExtRepo,
		c.cliffSvc,
		goreleaserSvc,
		c.fsRepo,
	)
	rootCmd.AddCommand(NewDryRunCmd(dryRunOrch))

	return nil
}
