package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultDockerSocketPath = "/var/run/docker.sock"

type dockerPrimaryEndpointRuntime struct {
	engine  dockerEngine
	project string
	service string
}

type dockerEngine interface {
	FindComposeContainer(project string, service string) (dockerContainerSummary, error)
	StartContainer(containerID string) error
	StopContainer(containerID string) error
}

type dockerContainerSummary struct {
	ID     string   `json:"Id"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
	Names  []string `json:"Names"`
}

type dockerEngineClient struct {
	httpClient *http.Client
}

func newDockerPrimaryEndpointRuntime(socketPath string, project string, service string) (primaryEndpointRuntime, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, fmt.Errorf("%w: compose project is required", ErrPrimaryEndpointUnavailable)
	}

	service = strings.TrimSpace(service)
	if service == "" {
		return nil, fmt.Errorf("%w: primary endpoint service is required", ErrPrimaryEndpointUnavailable)
	}

	engine, err := newDockerEngineClient(socketPath)
	if err != nil {
		return nil, err
	}

	return &dockerPrimaryEndpointRuntime{engine: engine, project: project, service: service}, nil
}

func (r *dockerPrimaryEndpointRuntime) Status() (primaryEndpointRuntimeStatus, error) {
	container, err := r.engine.FindComposeContainer(r.project, r.service)
	if err != nil {
		return primaryEndpointRuntimeStatus{}, err
	}

	state := strings.TrimSpace(container.State)
	if state == "" {
		state = "unknown"
	}
	running := state == "running"
	ready := running
	message := ""

	statusSummary := strings.TrimSpace(container.Status)
	statusLower := strings.ToLower(statusSummary)
	if running {
		switch {
		case strings.Contains(statusLower, "health: starting"):
			ready = false
			state = "starting"
			message = "container health check is starting"
		case strings.Contains(statusLower, "health: unhealthy"), strings.Contains(statusLower, "(unhealthy)"):
			ready = false
			state = "unhealthy"
			if statusSummary == "" {
				message = "container health check is unhealthy"
			} else {
				message = statusSummary
			}
		default:
			state = "running"
		}
	} else {
		ready = false
		if state == "unknown" {
			state = "stopped"
		}
		if statusSummary != "" {
			message = statusSummary
		} else {
			message = "container is not running"
		}
	}

	return primaryEndpointRuntimeStatus{
		Running: running,
		Ready:   ready,
		State:   state,
		Message: message,
	}, nil
}

func (r *dockerPrimaryEndpointRuntime) Start() error {
	container, err := r.engine.FindComposeContainer(r.project, r.service)
	if err != nil {
		return err
	}

	if container.State == "running" {
		return nil
	}

	return r.engine.StartContainer(container.ID)
}

func (r *dockerPrimaryEndpointRuntime) Stop() error {
	container, err := r.engine.FindComposeContainer(r.project, r.service)
	if err != nil {
		return err
	}

	if container.State != "running" {
		return nil
	}

	return r.engine.StopContainer(container.ID)
}

func newDockerEngineClient(socketPath string) (*dockerEngineClient, error) {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		socketPath = defaultDockerSocketPath
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: 10 * time.Second,
	}

	return &dockerEngineClient{httpClient: httpClient}, nil
}

func (c *dockerEngineClient) FindComposeContainer(project string, service string) (dockerContainerSummary, error) {
	filters := map[string][]string{
		"label": {
			fmt.Sprintf("com.docker.compose.project=%s", project),
			fmt.Sprintf("com.docker.compose.service=%s", service),
		},
	}

	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return dockerContainerSummary{}, fmt.Errorf("%w: encode docker filters: %v", ErrPrimaryEndpointUnavailable, err)
	}

	query := url.Values{}
	query.Set("all", "1")
	query.Set("filters", string(filtersJSON))

	req, err := http.NewRequest(http.MethodGet, "http://docker/containers/json?"+query.Encode(), nil)
	if err != nil {
		return dockerContainerSummary{}, fmt.Errorf("%w: build docker containers request: %v", ErrPrimaryEndpointUnavailable, err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return dockerContainerSummary{}, fmt.Errorf("%w: query docker containers: %v", ErrPrimaryEndpointUnavailable, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body := readResponseBody(res.Body)
		return dockerContainerSummary{}, fmt.Errorf("%w: docker containers request failed with status %d: %s", ErrPrimaryEndpointUnavailable, res.StatusCode, body)
	}

	var containers []dockerContainerSummary
	if err := json.NewDecoder(res.Body).Decode(&containers); err != nil {
		return dockerContainerSummary{}, fmt.Errorf("%w: decode docker containers response: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if len(containers) == 0 {
		return dockerContainerSummary{}, fmt.Errorf("%w: service=%s project=%s", ErrPrimaryEndpointNotFound, service, project)
	}

	for _, container := range containers {
		if container.State == "running" {
			return container, nil
		}
	}

	return containers[0], nil
}

func (c *dockerEngineClient) StartContainer(containerID string) error {
	req, err := http.NewRequest(http.MethodPost, "http://docker/containers/"+containerID+"/start", nil)
	if err != nil {
		return fmt.Errorf("%w: build docker start request: %v", ErrPrimaryEndpointUnavailable, err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: start docker container: %v", ErrPrimaryEndpointUnavailable, err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified || res.StatusCode == http.StatusNoContent {
		return nil
	}

	if res.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: container=%s", ErrPrimaryEndpointNotFound, containerID)
	}

	body := readResponseBody(res.Body)
	return fmt.Errorf("%w: docker start failed with status %d: %s", ErrPrimaryEndpointUnavailable, res.StatusCode, body)
}

func (c *dockerEngineClient) StopContainer(containerID string) error {
	req, err := http.NewRequest(http.MethodPost, "http://docker/containers/"+containerID+"/stop?t=10", nil)
	if err != nil {
		return fmt.Errorf("%w: build docker stop request: %v", ErrPrimaryEndpointUnavailable, err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: stop docker container: %v", ErrPrimaryEndpointUnavailable, err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified || res.StatusCode == http.StatusNoContent {
		return nil
	}

	if res.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: container=%s", ErrPrimaryEndpointNotFound, containerID)
	}

	body := readResponseBody(res.Body)
	return fmt.Errorf("%w: docker stop failed with status %d: %s", ErrPrimaryEndpointUnavailable, res.StatusCode, body)
}

func readResponseBody(body io.Reader) string {
	payload, err := io.ReadAll(body)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(payload))
}

var _ dockerEngine = (*dockerEngineClient)(nil)
var _ primaryEndpointRuntime = (*dockerPrimaryEndpointRuntime)(nil)
