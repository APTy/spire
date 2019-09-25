package docker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func requireFind(t *testing.T, cf ContainerIDFinder, cgroup, expectedID string) {
	t.Helper()
	id, ok := cf.FindContainerID(cgroup)
	require.True(t, ok, "expected to find %q but didn't", cgroup)
	require.Equal(t, expectedID, id)
}

func requireNotFind(t *testing.T, cf ContainerIDFinder, cgroups ...string) {
	t.Helper()

	for _, cgroup := range cgroups {
		id, ok := cf.FindContainerID(cgroup)
		require.False(t, ok, "expected to not find %q but did", cgroup)
		require.Equal(t, "", id)
	}
}

func TestContainerIDFetcher(t *testing.T) {
	cf, err := NewContainerIDFetcher("/docker/<id>")
	require.NoError(t, err)

	requireFind(t, cf, "/docker/", "")
	requireFind(t, cf, "/docker/foo", "foo")
	requireNotFind(t, cf,
		"",
		"/",
		"/docker",
		"/dockerfoo",
		"/docker/foo/",
		"/docker/foo/bar",
		"/docker/foo/docker/foo",
	)
}

func TestContainerIDFetcher2(t *testing.T) {
	cf, err := NewContainerIDFetcher("/my.slice/*/<id>")
	require.NoError(t, err)

	requireFind(t, cf, "/my.slice/foo/", "")
	requireFind(t, cf, "/my.slice/foo/bar", "bar")
	requireNotFind(t, cf,
		"",
		"/",
		"/my.slice",
		"/my.slicefoo",
		"/my.slice/foo",
		"/my.slice/foo/my.slice/foo",
	)
}

func TestContainerIDFetcher3(t *testing.T) {
	cf, err := NewContainerIDFetcher("/long.slice/*/*/<id>/*")
	require.NoError(t, err)

	requireFind(t, cf, "/long.slice/foo/bar//qux", "")
	requireFind(t, cf, "/long.slice/foo/bar/baz/", "baz")
	requireFind(t, cf, "/long.slice/foo/bar/baz/qux", "baz")
	requireNotFind(t, cf,
		"",
		"/",
		"/long.slice",
		"/long.slicefoo",
		"/long.slice/foo",
		"/long.slice/foo/",
		"/long.slice/foo/bar",
		"/long.slice/foo/long.slice/foo",
		"/long.slice/foo/bar/baz",
		"/long.slice/foo/bar/baz/qux/qax",
	)
}

func TestNewContainerIDFetcherErrors(t *testing.T) {
	tests := []struct {
		desc      string
		pattern   string
		expectErr string
	}{
		{
			desc:      "no <id> token",
			pattern:   "/foo/bar",
			expectErr: `pattern "/foo/bar" must contain the container id token "<id>" exactly once`,
		},
		{
			desc:      "more than one <id> token",
			pattern:   "/foo/<id>/bar/<id>",
			expectErr: `pattern "/foo/<id>/bar/<id>" must contain the container id token "<id>" exactly once`,
		},
		{
			desc:      "bad regexp characters",
			pattern:   "\\<id>",
			expectErr: `failed to create container id fetcher: error parsing regexp`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, err := NewContainerIDFetcher(tt.pattern)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectErr)
		})
	}
}
