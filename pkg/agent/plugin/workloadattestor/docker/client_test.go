package docker

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDockerContainerInspect(t *testing.T) {
	tests := []struct {
		desc                    string
		socketPath              string
		httpResponseFixtureFile string
		httpResponseStatusCode  int
		requireExpectedResponse func(t *testing.T, info *ContainerInfo)
		expectErr               string
	}{
		{
			desc:                    "good response",
			httpResponseFixtureFile: "../../../../../test/fixture/workloadattestor/docker/container.json",
			httpResponseStatusCode:  http.StatusOK,
			requireExpectedResponse: func(t *testing.T, info *ContainerInfo) {
				require.Equal(t, "debian", info.Config.Image)
				require.Equal(t, "foo", info.Config.Labels["com.example.name"])
				require.Equal(t, "test", info.Config.Labels["com.example.env"])
			},
		},
		{
			desc:                    "good response no labels",
			httpResponseFixtureFile: "../../../../../test/fixture/workloadattestor/docker/container-no-labels.json",
			httpResponseStatusCode:  http.StatusOK,
			requireExpectedResponse: func(t *testing.T, info *ContainerInfo) {
				require.Equal(t, "debian", info.Config.Image)
				require.Empty(t, info.Config.Labels)
			},
		},
		{
			desc:                    "not found response",
			httpResponseStatusCode:  http.StatusNotFound,
			httpResponseFixtureFile: "../../../../../test/fixture/workloadattestor/docker/container-not-found.json",
			expectErr:               "No such container: abcdef",
		},
		{
			desc:                    "server error response",
			httpResponseStatusCode:  http.StatusInternalServerError,
			httpResponseFixtureFile: "../../../../../test/fixture/workloadattestor/docker/container-server-error.json",
			expectErr:               "Something went wrong.",
		},
		{
			desc:                    "bad version response",
			httpResponseStatusCode:  http.StatusBadRequest,
			httpResponseFixtureFile: "../../../../../test/fixture/workloadattestor/docker/response-bad-version.txt",
			expectErr:               "client version 1.10 is too old. Minimum supported API version is 1.12, please upgrade your client to a newer version",
		},
		{
			desc:       "bad socket path",
			socketPath: "doesnt-exist",
			expectErr:  "dial unix doesnt-exist: connect: no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if tt.socketPath == "" {
				tt.socketPath = "test-socket-path"
			}
			testVersion := "1.30"
			// ensure there aren't any lingering sockets
			os.Remove(tt.socketPath)

			handler := http.NewServeMux()
			handler.HandleFunc("/v1.30/containers/abcdef/json", func(w http.ResponseWriter, r *http.Request) {
				f, err := os.Open(tt.httpResponseFixtureFile)
				require.NoError(t, err)
				w.WriteHeader(tt.httpResponseStatusCode)
				_, err = io.Copy(w, f)
				require.NoError(t, err)
			})
			server := http.Server{Handler: handler}
			defer server.Close()

			listener, err := net.Listen("unix", "test-socket-path")
			require.NoError(t, err)
			go server.Serve(listener)

			client := newDockerClient(tt.socketPath, testVersion)
			info, err := client.ContainerInspect(context.Background(), "abcdef")
			if tt.expectErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, info)
			tt.requireExpectedResponse(t, info)
		})
	}
}
