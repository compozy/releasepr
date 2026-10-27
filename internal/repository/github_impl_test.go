package repository

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v74/github"
	"github.com/stretchr/testify/require"
)

func TestGithubRepository_ListOpenPRs(t *testing.T) {
	t.Run("Should follow pagination and distinguish same repository heads from forks", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			require.Equal(t, "/repos/compozy/releasepr/pulls", r.URL.Path)
			require.Equal(t, "open", r.URL.Query().Get("state"))
			require.Equal(t, "main", r.URL.Query().Get("base"))
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "" {
				w.Header().
					Set("Link", fmt.Sprintf(`<http://%s/repos/compozy/releasepr/pulls?state=open&base=main&page=2>; rel="next"`, r.Host))
				_, err := fmt.Fprint(
					w,
					`[{"number":1,"html_url":"https://github.com/compozy/releasepr/pull/1","title":"build: release v1.0.0","head":{"ref":"release/v1.0.0","repo":{"full_name":"compozy/releasepr"}},"labels":[{"name":"release-pending"}]}]`,
				)
				require.NoError(t, err)
				return
			}
			_, err := fmt.Fprint(
				w,
				`[{"number":2,"head":{"ref":"release/v0.9.0","repo":{"full_name":"outsider/releasepr"}}},{"number":3}]`,
			)
			require.NoError(t, err)
		}))
		defer server.Close()
		client := github.NewClient(server.Client())
		base, err := url.Parse(server.URL + "/")
		require.NoError(t, err)
		client.BaseURL = base
		repo := &githubRepository{client: client, owner: "compozy", repo: "releasepr"}
		prs, err := repo.ListOpenPRs(t.Context(), "main")
		require.NoError(t, err)
		require.Len(t, prs, 3)
		require.Equal(t, 2, calls)
		require.True(t, prs[0].SameRepository)
		require.Equal(t, []string{"release-pending"}, prs[0].Labels)
		require.Equal(t, "release/v1.0.0", prs[0].Head)
		require.False(t, prs[1].SameRepository)
		require.False(t, prs[2].SameRepository)
	})
}
