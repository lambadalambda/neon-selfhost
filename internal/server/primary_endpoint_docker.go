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

const (
	defaultDockerSocketPath  = "/var/run/docker.sock"
	dockerEngineHTTPTimeout  = 30 * time.Second
	dockerContainerStopGrace = 10
)

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

type dockerContainerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image string `json:"Image"`
	} `json:"Config"`
	State struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
	} `json:"State"`
}

type dockerCreateContainerRequest struct {
	Name        string
	Image       string
	Entrypoint  []string
	Env         []string
	Labels      map[string]string
	HostConfig  dockerCreateHostConfig
	Networking  map[string]dockerCreateEndpointSettings
	AutoRemove  bool
	WorkingDir  string
	User        string
	AttachStdin bool
}

type dockerCreateHostConfig struct {
	NetworkMode string              `json:"NetworkMode,omitempty"`
	Mounts      []dockerMountConfig `json:"Mounts,omitempty"`
	AutoRemove  bool                `json:"AutoRemove,omitempty"`
}

type dockerMountConfig struct {
	Type     string `json:"Type"`
	Source   string `json:"Source"`
	Target   string `json:"Target"`
	ReadOnly bool   `json:"ReadOnly,omitempty"`
}

type dockerCreateEndpointSettings struct {
	Aliases []string `json:"Aliases,omitempty"`
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
		Timeout: dockerEngineHTTPTimeout,
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
	stopURL := fmt.Sprintf("http://docker/containers/%s/stop?t=%d", containerID, dockerContainerStopGrace)
	req, err := http.NewRequest(http.MethodPost, stopURL, nil)
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

func (c *dockerEngineClient) InspectContainerByName(name string) (dockerContainerInspect, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return dockerContainerInspect{}, false, fmt.Errorf("%w: container name is required", ErrPrimaryEndpointUnavailable)
	}

	req, err := http.NewRequest(http.MethodGet, "http://docker/containers/"+url.PathEscape(name)+"/json", nil)
	if err != nil {
		return dockerContainerInspect{}, false, fmt.Errorf("%w: build docker inspect request: %v", ErrPrimaryEndpointUnavailable, err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return dockerContainerInspect{}, false, fmt.Errorf("%w: inspect docker container: %v", ErrPrimaryEndpointUnavailable, err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return dockerContainerInspect{}, false, nil
	}

	if res.StatusCode != http.StatusOK {
		body := readResponseBody(res.Body)
		return dockerContainerInspect{}, false, fmt.Errorf("%w: docker inspect failed with status %d: %s", ErrPrimaryEndpointUnavailable, res.StatusCode, body)
	}

	var payload dockerContainerInspect
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return dockerContainerInspect{}, false, fmt.Errorf("%w: decode docker inspect response: %v", ErrPrimaryEndpointUnavailable, err)
	}

	return payload, true, nil
}

func (c *dockerEngineClient) CreateContainer(reqSpec dockerCreateContainerRequest) (string, error) {
	if strings.TrimSpace(reqSpec.Name) == "" {
		return "", fmt.Errorf("%w: container name is required", ErrPrimaryEndpointUnavailable)
	}

	if strings.TrimSpace(reqSpec.Image) == "" {
		return "", fmt.Errorf("%w: container image is required", ErrPrimaryEndpointUnavailable)
	}

	payload := struct {
		Image            string                               `json:"Image"`
		Entrypoint       []string                             `json:"Entrypoint,omitempty"`
		Env              []string                             `json:"Env,omitempty"`
		Labels           map[string]string                    `json:"Labels,omitempty"`
		HostConfig       dockerCreateHostConfig               `json:"HostConfig,omitempty"`
		WorkingDir       string                               `json:"WorkingDir,omitempty"`
		User             string                               `json:"User,omitempty"`
		AttachStdin      bool                                 `json:"AttachStdin,omitempty"`
		NetworkingConfig *dockerCreateNetworkingConfigPayload `json:"NetworkingConfig,omitempty"`
	}{
		Image:       reqSpec.Image,
		Entrypoint:  reqSpec.Entrypoint,
		Env:         reqSpec.Env,
		Labels:      reqSpec.Labels,
		HostConfig:  reqSpec.HostConfig,
		WorkingDir:  reqSpec.WorkingDir,
		User:        reqSpec.User,
		AttachStdin: reqSpec.AttachStdin,
	}

	if len(reqSpec.Networking) > 0 {
		payload.NetworkingConfig = &dockerCreateNetworkingConfigPayload{EndpointsConfig: reqSpec.Networking}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: encode docker create request: %v", ErrPrimaryEndpointUnavailable, err)
	}

	query := url.Values{}
	query.Set("name", reqSpec.Name)

	httpReq, err := http.NewRequest(http.MethodPost, "http://docker/containers/create?"+query.Encode(), strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("%w: build docker create request: %v", ErrPrimaryEndpointUnavailable, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: create docker container: %v", ErrPrimaryEndpointUnavailable, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		body := readResponseBody(res.Body)
		return "", fmt.Errorf("%w: docker create failed with status %d: %s", ErrPrimaryEndpointUnavailable, res.StatusCode, body)
	}

	var createPayload struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&createPayload); err != nil {
		return "", fmt.Errorf("%w: decode docker create response: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if strings.TrimSpace(createPayload.ID) == "" {
		return "", fmt.Errorf("%w: docker create returned empty container id", ErrPrimaryEndpointUnavailable)
	}

	return createPayload.ID, nil
}

type dockerCreateNetworkingConfigPayload struct {
	EndpointsConfig map[string]dockerCreateEndpointSettings `json:"EndpointsConfig"`
}

func (c *dockerEngineClient) RemoveContainer(containerID string, force bool) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return nil
	}

	query := url.Values{}
	if force {
		query.Set("force", "1")
	}

	urlPath := "http://docker/containers/" + url.PathEscape(containerID)
	if encoded := query.Encode(); encoded != "" {
		urlPath += "?" + encoded
	}

	req, err := http.NewRequest(http.MethodDelete, urlPath, nil)
	if err != nil {
		return fmt.Errorf("%w: build docker remove request: %v", ErrPrimaryEndpointUnavailable, err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: remove docker container: %v", ErrPrimaryEndpointUnavailable, err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusNotFound {
		return nil
	}

	body := readResponseBody(res.Body)
	return fmt.Errorf("%w: docker remove failed with status %d: %s", ErrPrimaryEndpointUnavailable, res.StatusCode, body)
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
