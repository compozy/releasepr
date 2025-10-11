package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/compozy/releasepr/internal/logger"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/viper"
)

type Config struct {
	GithubToken           string `mapstructure:"github_token"`
	GithubOwner           string `mapstructure:"github_owner"`
	GithubRepo            string `mapstructure:"github_repo"`
	ToolsDir              string `mapstructure:"tools_dir"`
	NpmToken              string `mapstructure:"npm_token"`
	LogLevel              string `mapstructure:"log_level"`
	LogFormat             string `mapstructure:"log_format"`
	GitPushTimeoutMinutes int    `mapstructure:"git_push_timeout_minutes"`
}

var configFileCandidates = []string{".pr-release", ".compozy-release"}

func DefaultConfig() *Config {
	logFormat := "json"
	if isCI() {
		logFormat = "console"
	}
	return &Config{
		ToolsDir:              "tools",
		LogLevel:              "info",
		LogFormat:             logFormat,
		GitPushTimeoutMinutes: 2,
	}
}

func isCI() bool {
	ciEnvVars := []string{
		"CI",
		"CONTINUOUS_INTEGRATION",
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"CIRCLECI",
		"TRAVIS",
		"JENKINS_URL",
		"BUILDKITE",
		"DRONE",
		"TEAMCITY_VERSION",
	}
	for _, envVar := range ciEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}
	return false
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
	if err := validateLogLevel(c.LogLevel); err != nil {
		return err
	}
	if err := validateLogFormat(c.LogFormat); err != nil {
		return err
	}
	if c.GitPushTimeoutMinutes < 1 || c.GitPushTimeoutMinutes > 30 {
		return fmt.Errorf("git_push_timeout_minutes must be between 1 and 30, got %d", c.GitPushTimeoutMinutes)
	}
	return nil
}

func (c *Config) LoggerConfig() logger.Config {
	return logger.Config{Level: c.LogLevel, Format: c.LogFormat}
}

func validateLogLevel(level string) error {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "info", "warn", "error":
		return nil
	}
	return fmt.Errorf("invalid log_level: %s", level)
}

func validateLogFormat(format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json", "console":
		return nil
	}
	return fmt.Errorf("invalid log_format: %s", format)
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

func bindEnvironmentVariables(v *viper.Viper) error {
	bindings := map[string][]string{
		"github_token": {
			"GITHUB_TOKEN",
			"PR_RELEASE_GITHUB_TOKEN",
			"COMPOZY_RELEASE_GITHUB_TOKEN",
			"RELEASE_TOKEN",
		},
		"github_owner": {"GITHUB_OWNER", "PR_RELEASE_GITHUB_OWNER", "COMPOZY_RELEASE_GITHUB_OWNER"},
		"github_repo":  {"GITHUB_REPO", "PR_RELEASE_GITHUB_REPO", "COMPOZY_RELEASE_GITHUB_REPO"},
		"tools_dir":    {"TOOLS_DIR", "PR_RELEASE_TOOLS_DIR", "COMPOZY_RELEASE_TOOLS_DIR"},
		"log_level":    {"LOG_LEVEL", "PR_RELEASE_LOG_LEVEL", "COMPOZY_RELEASE_LOG_LEVEL"},
		"log_format":   {"LOG_FORMAT", "PR_RELEASE_LOG_FORMAT", "COMPOZY_RELEASE_LOG_FORMAT"},
		"npm_token":    {"NPM_TOKEN", "PR_RELEASE_NPM_TOKEN", "COMPOZY_RELEASE_NPM_TOKEN"},
		"git_push_timeout_minutes": {
			"GIT_PUSH_TIMEOUT_MINUTES",
			"PR_RELEASE_GIT_PUSH_TIMEOUT_MINUTES",
			"COMPOZY_RELEASE_GIT_PUSH_TIMEOUT_MINUTES",
		},
	}
	for key, envs := range bindings {
		if err := v.BindEnv(append([]string{key}, envs...)...); err != nil {
			return fmt.Errorf("failed to bind %s env: %w", key, err)
		}
	}
	return nil
}

func setConfigDefaults(v *viper.Viper) {
	defaults := DefaultConfig()
	v.SetDefault("tools_dir", defaults.ToolsDir)
	v.SetDefault("log_level", defaults.LogLevel)
	v.SetDefault("log_format", defaults.LogFormat)
	v.SetDefault("git_push_timeout_minutes", defaults.GitPushTimeoutMinutes)
}

func LoadConfig() (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := bindEnvironmentVariables(v); err != nil {
		return nil, err
	}
	setConfigDefaults(v)
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
