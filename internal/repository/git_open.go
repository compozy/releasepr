package repository

import "github.com/go-git/go-git/v5"

func openGitRepository() (*git.Repository, error) {
	return git.PlainOpenWithOptions(".", &git.PlainOpenOptions{
		EnableDotGitCommonDir: true,
	})
}
