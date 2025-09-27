package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/require"
)

func TestPopulateRepositoryDefaultsUsesEnvSlug(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "acme/widgets")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "")
	t.Setenv("GITHUB_REPOSITORY_NAME", "")
	cfg := Config{}
	err := populateRepositoryDefaults(&cfg)
	require.NoError(t, err)
	require.Equal(t, "acme", cfg.GithubOwner)
	require.Equal(t, "widgets", cfg.GithubRepo)
}

func TestPopulateRepositoryDefaultsFallsBackToGitRemote(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "")
	t.Setenv("GITHUB_REPOSITORY_NAME", "")
	tmp := t.TempDir()
	repo, err := git.PlainInit(tmp, false)
	require.NoError(t, err)
	_, err = repo.CreateRemote(
		&gitconfig.RemoteConfig{Name: "origin", URLs: []string{"git@github.com:octo/widget.git"}},
	)
	require.NoError(t, err)
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { require.NoError(t, os.Chdir(wd)) })
	cfg := Config{}
	err = populateRepositoryDefaults(&cfg)
	require.NoError(t, err)
	require.Equal(t, "octo", cfg.GithubOwner)
	require.Equal(t, "widget", cfg.GithubRepo)
}

func TestParseGitRemoteURL(t *testing.T) {
	cases := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
	}{
		{name: "https clone", url: "https://github.com/org/project.git", wantOwner: "org", wantRepo: "project"},
		{name: "ssh", url: "git@github.com:org/project.git", wantOwner: "org", wantRepo: "project"},
		{name: "ssh without suffix", url: "git@github.com:org/project", wantOwner: "org", wantRepo: "project"},
		{name: "file path", url: filepath.Join("tmp", "org", "project"), wantOwner: "org", wantRepo: "project"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := parseGitRemoteURL(tc.url)
			require.NoError(t, err)
			require.Equal(t, tc.wantOwner, owner)
			require.Equal(t, tc.wantRepo, repo)
		})
	}
}
