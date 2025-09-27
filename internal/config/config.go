package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/spf13/viper"
)

type Config struct {
	GithubToken string `mapstructure:"github_token"`
	GithubOwner string `mapstructure:"github_owner"`
	GithubRepo  string `mapstructure:"github_repo"`
	ToolsDir    string `mapstructure:"tools_dir"`
	NpmToken    string `mapstructure:"npm_token"`
}

var configFileCandidates = []string{".pr-release", ".compozy-release"}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{ToolsDir: "tools"}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GithubToken != "" {
		if err := ValidateGitHubToken(c.GithubToken); err != nil {
			return fmt.Errorf("invalid github_token: %w", err)
		}
	}
	if err := ValidateGitHubOwnerRepo(c.GithubOwner, c.GithubRepo); err != nil {
		return fmt.Errorf("invalid github configuration: %w", err)
	}
	if c.ToolsDir == "" {
		return fmt.Errorf("tools_dir cannot be empty")
	}
	if strings.Contains(c.ToolsDir, "..") {
		return fmt.Errorf("tools_dir contains invalid path traversal")
	}
	return nil
}

// ValidateForGitHubOperations validates that GitHub token is present for operations that require it.
func (c *Config) ValidateForGitHubOperations() error {
	if c.GithubToken == "" {
		return fmt.Errorf("github_token is required for GitHub operations")
	}
	return c.Validate()
}

// ValidateGitHubToken validates GitHub token format (exported for reuse).
func ValidateGitHubToken(token string) error {
	token = strings.TrimSpace(token)
	classicPAT := regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
	fineGrainedPAT := regexp.MustCompile(`^github_pat_[a-zA-Z0-9_]{82}$`)
	appToken := regexp.MustCompile(`^ghs_[a-zA-Z0-9]{36}$`)
	oauthToken := regexp.MustCompile(`^gho_[a-zA-Z0-9]{36}$`)
	if !classicPAT.MatchString(token) &&
		!fineGrainedPAT.MatchString(token) &&
		!appToken.MatchString(token) &&
		!oauthToken.MatchString(token) {
		return fmt.Errorf("invalid token format")
	}
	return nil
}

// ValidateGitHubOwnerRepo validates GitHub owner and repository names (exported for reuse).
func ValidateGitHubOwnerRepo(owner, repo string) error {
	if owner == "" {
		return fmt.Errorf("owner cannot be empty")
	}
	if repo == "" {
		return fmt.Errorf("repository cannot be empty")
	}
	validName := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-_.]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)
	if !validName.MatchString(owner) {
		return fmt.Errorf("invalid owner format: %s", owner)
	}
	if len(owner) > 39 {
		return fmt.Errorf("owner too long: maximum 39 characters")
	}
	if !validName.MatchString(repo) {
		return fmt.Errorf("invalid repository format: %s", repo)
	}
	if len(repo) > 100 {
		return fmt.Errorf("repository too long: maximum 100 characters")
	}
	return nil
}

func LoadConfig() (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := v.BindEnv(
		"github_token",
		"GITHUB_TOKEN",
		"PR_RELEASE_GITHUB_TOKEN",
		"COMPOZY_RELEASE_GITHUB_TOKEN",
		"RELEASE_TOKEN",
	); err != nil {
		return nil, fmt.Errorf("failed to bind github_token env: %w", err)
	}
	if err := v.BindEnv(
		"github_owner",
		"GITHUB_OWNER",
		"PR_RELEASE_GITHUB_OWNER",
		"COMPOZY_RELEASE_GITHUB_OWNER",
	); err != nil {
		return nil, fmt.Errorf("failed to bind github_owner env: %w", err)
	}
	if err := v.BindEnv(
		"github_repo",
		"GITHUB_REPO",
		"PR_RELEASE_GITHUB_REPO",
		"COMPOZY_RELEASE_GITHUB_REPO",
	); err != nil {
		return nil, fmt.Errorf("failed to bind github_repo env: %w", err)
	}
	if err := v.BindEnv("tools_dir", "TOOLS_DIR", "PR_RELEASE_TOOLS_DIR", "COMPOZY_RELEASE_TOOLS_DIR"); err != nil {
		return nil, fmt.Errorf("failed to bind tools_dir env: %w", err)
	}
	if err := v.BindEnv("npm_token", "NPM_TOKEN", "PR_RELEASE_NPM_TOKEN", "COMPOZY_RELEASE_NPM_TOKEN"); err != nil {
		return nil, fmt.Errorf("failed to bind npm_token env: %w", err)
	}
	defaults := DefaultConfig()
	v.SetDefault("tools_dir", defaults.ToolsDir)
	for _, name := range configFileCandidates {
		v.SetConfigName(name)
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				continue
			}
			return nil, err
		}
		break
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if err := populateRepositoryDefaults(&cfg); err != nil {
		return nil, fmt.Errorf("repository detection failed: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	return &cfg, nil
}

func populateRepositoryDefaults(cfg *Config) error {
	owner := strings.TrimSpace(cfg.GithubOwner)
	repo := strings.TrimSpace(cfg.GithubRepo)
	owner, repo = applyRepositoryEnvFallbacks(owner, repo)
	if owner != "" && repo != "" {
		cfg.GithubOwner = owner
		cfg.GithubRepo = repo
		return nil
	}
	gitOwner, gitRepo, err := inferRepoFromGitRemote()
	if err == nil {
		if owner == "" {
			owner = gitOwner
		}
		if repo == "" {
			repo = gitRepo
		}
	}
	cfg.GithubOwner = owner
	cfg.GithubRepo = repo
	if cfg.GithubOwner == "" || cfg.GithubRepo == "" {
		return fmt.Errorf("unable to determine GitHub owner/repo; set via config or environment")
	}
	return nil
}

func applyRepositoryEnvFallbacks(owner, repo string) (string, string) {
	slug := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY"))
	if slug != "" {
		parsedOwner, parsedRepo, err := parseRepoSlug(slug)
		if err == nil {
			if owner == "" {
				owner = parsedOwner
			}
			if repo == "" {
				repo = parsedRepo
			}
		}
	}
	if owner == "" {
		owner = strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY_OWNER"))
	}
	if repo == "" {
		repo = strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY_NAME"))
	}
	return owner, repo
}

func inferRepoFromGitRemote() (string, string, error) {
	repo, err := git.PlainOpen(".")
	if err != nil {
		return "", "", err
	}
	remote, err := repo.Remote("origin")
	if err != nil {
		return "", "", err
	}
	urls := remote.Config().URLs
	if len(urls) == 0 {
		return "", "", fmt.Errorf("origin remote has no URLs")
	}
	for _, remoteURL := range urls {
		owner, name, parseErr := parseGitRemoteURL(remoteURL)
		if parseErr == nil && owner != "" && name != "" {
			return owner, name, nil
		}
	}
	return "", "", fmt.Errorf("could not determine repository from remote")
}

func parseRepoSlug(slug string) (string, string, error) {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository slug: %s", slug)
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("invalid repository slug: %s", slug)
	}
	return owner, repo, nil
}

func parseGitRemoteURL(remoteURL string) (string, string, error) {
	trimmed := strings.TrimSuffix(remoteURL, ".git")
	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", "", err
		}
		path := strings.TrimPrefix(u.Path, "/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid remote path: %s", remoteURL)
		}
		return parts[0], parts[1], nil
	}
	if strings.Contains(trimmed, ":") {
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid remote path: %s", remoteURL)
		}
		path := strings.TrimPrefix(parts[1], "/")
		segments := strings.SplitN(path, "/", 2)
		if len(segments) != 2 {
			return "", "", fmt.Errorf("invalid remote path: %s", remoteURL)
		}
		return segments[0], segments[1], nil
	}
	segments := strings.Split(trimmed, "/")
	if len(segments) < 2 {
		return "", "", fmt.Errorf("invalid remote path: %s", remoteURL)
	}
	owner := segments[len(segments)-2]
	name := segments[len(segments)-1]
	return owner, name, nil
}
