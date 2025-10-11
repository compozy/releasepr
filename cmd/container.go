package cmd

import (
	"fmt"
	"os"

	"github.com/compozy/releasepr/internal/config"
	"github.com/compozy/releasepr/internal/orchestrator"
	"github.com/compozy/releasepr/internal/repository"
	"github.com/compozy/releasepr/internal/service"
	"github.com/spf13/afero"
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

	// Individual commands have been replaced by orchestrator commands

	// Add orchestrator-based commands
	if err := addOrchestratorCommands(c); err != nil {
		return err
	}

	rootCmd.AddCommand(newVersionCmd())

	return nil
}

// addOrchestratorCommands adds the new consolidated commands
func addOrchestratorCommands(c *container) error {
	// Initialize extended repositories for orchestrators
	gitExtRepo, err := repository.NewGitExtendedRepository()
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
	fmt.Printf("GitHub configuration: owner=%s, repo=%s, token_source=%s, token_present=%t, token_length=%d\n",
		owner, repo, tokenSource, token != "", len(token))
	if owner == "" || repo == "" {
		return fmt.Errorf("github owner/repo not configured; set GITHUB_REPOSITORY or config values")
	}
	var githubExtRepo repository.GithubExtendedRepository
	if token == "" {
		fmt.Fprintln(os.Stderr, "GitHub token not provided; GitHub operations will be skipped")
		githubExtRepo = repository.NewGithubNoopExtendedRepository(owner, repo)
	} else {
		fmt.Printf("Initializing GitHub extended repository with token (length=%d)\n", len(token))
		var err error
		githubExtRepo, err = repository.NewGithubExtendedRepository(token, owner, repo)
		if err != nil {
			return fmt.Errorf("failed to initialize GitHub extended repository: %w", err)
		}
		fmt.Printf("Successfully initialized GitHub extended repository for %s/%s\n", owner, repo)
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
