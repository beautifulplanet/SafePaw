// =============================================================
// SafePaw Wizard - Docker Engine API Client
// =============================================================
// Lightweight Docker client that talks to the Engine API over a
// Unix socket (or named pipe on Windows). Zero external deps —
// just net/http with a custom dialer.
//
// We only need a handful of endpoints:
//   GET  /_ping                    — Daemon reachability
//   GET  /containers/json          — List containers (filtered)
//   GET  /containers/{id}/json     — Inspect one container
//   POST /containers/{id}/restart  — Restart a service
//
// All requests target Docker Engine API v1.43 for broad compat
// (Docker 24.0+, released June 2023).
// =============================================================

package docker

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

const apiVersion = "v1.43"

// Client communicates with the Docker daemon via the Engine API.
type Client struct {
	http    *http.Client
	project string // Compose project name for filtering
	baseURL string // http://docker (unix) or http://host:port (tcp)
}

// Container represents a Docker container summary from the list endpoint.
type Container struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`  // "running", "exited", "created", etc.
	Status  string            `json:"Status"` // Human-readable: "Up 2 hours (healthy)"
	Labels  map[string]string `json:"Labels"`
	Created int64             `json:"Created"`
}

// ContainerDetail holds the full inspect response (subset we care about).
type ContainerDetail struct {
	ID    string         `json:"Id"`
	Name  string         `json:"Name"`
	State ContainerState `json:"State"`
}

// ContainerState is the runtime state from inspect.
type ContainerState struct {
	Status     string       `json:"Status"` // "running", "exited", etc.
	Running    bool         `json:"Running"`
	StartedAt  string       `json:"StartedAt"`
	FinishedAt string       `json:"FinishedAt"`
	Health     *HealthState `json:"Health,omitempty"`
}

// HealthState is the container health check result.
type HealthState struct {
	Status string `json:"Status"` // "healthy", "unhealthy", "starting", "none"
}

// ServiceInfo is a simplified view for the wizard UI.
type ServiceInfo struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	State  string `json:"state"`  // "running", "stopped", "error"
	Health string `json:"health"` // "healthy", "unhealthy", "starting", "none"
	Image  string `json:"image"`
	Uptime string `json:"uptime,omitempty"`
}

// New creates a Docker client connected to the daemon.
// host can be "unix:///var/run/docker.sock" or "tcp://host:port".
// project is the Compose project name used to filter containers (e.g. "safepaw").
func New(host, project string) *Client {
	var transport *http.Transport
	var baseURL string

	if strings.HasPrefix(host, "tcp://") {
		// TCP connection (e.g. docker-socket-proxy)
		addr := strings.TrimPrefix(host, "tcp://")
		transport = &http.Transport{
			MaxIdleConns:    5,
			IdleConnTimeout: 30 * time.Second,
		}
		baseURL = "http://" + addr
	} else {
		// Unix socket connection
		socketPath := strings.TrimPrefix(host, "unix://")
		transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
			},
			MaxIdleConns:    5,
			IdleConnTimeout: 30 * time.Second,
		}
		baseURL = "http://docker"
	}

	return &Client{
		http: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
		project: project,
		baseURL: baseURL,
	}
}

// Ping checks if the Docker daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.do(ctx, "GET", "/_ping", nil)
	if err != nil {
		return fmt.Errorf("docker daemon unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// ListContainers returns all containers for the Compose project (including stopped).
func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	// Filter by our compose project label
	filters := fmt.Sprintf(`{"label":["com.docker.compose.project=%s"]}`, c.project)

	path := fmt.Sprintf("/%s/containers/json?all=true&filters=%s",
		apiVersion, url.QueryEscape(filters))

	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	defer resp.Body.Close()

	var containers []Container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("failed to decode container list: %w", err)
	}
	return containers, nil
}

// InspectContainer returns detailed info about a container by ID or name.
func (c *Client) InspectContainer(ctx context.Context, nameOrID string) (*ContainerDetail, error) {
	path := fmt.Sprintf("/%s/containers/%s/json", apiVersion, nameOrID)

	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", nameOrID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("container %s not found", nameOrID)
	}

	var detail ContainerDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("failed to decode container detail: %w", err)
	}
	return &detail, nil
}

// RestartContainer restarts a container with a timeout (seconds).
func (c *Client) RestartContainer(ctx context.Context, nameOrID string, timeoutSecs int) error {
	path := fmt.Sprintf("/%s/containers/%s/restart?t=%d", apiVersion, nameOrID, timeoutSecs)

	resp, err := c.do(ctx, "POST", path, nil)
	if err != nil {
		return fmt.Errorf("failed to restart container %s: %w", nameOrID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("restart %s returned %d: %s", nameOrID, resp.StatusCode, string(body))
	}
	return nil
}

// Services returns a simplified list of all SafePaw services with health info.
func (c *Client) Services(ctx context.Context) ([]ServiceInfo, error) {
	containers, err := c.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	services := make([]ServiceInfo, 0, len(containers))
	for _, ct := range containers {
		svc := ServiceInfo{
			ID:    ct.ID[:12], // Short ID
			State: ct.State,
			Image: ct.Image,
		}

		// Extract compose service name from labels
		if name, ok := ct.Labels["com.docker.compose.service"]; ok {
			svc.Name = name
		} else if len(ct.Names) > 0 {
			svc.Name = strings.TrimPrefix(ct.Names[0], "/")
		}

		// Get health status via inspect (list endpoint doesn't include it)
		if detail, err := c.InspectContainer(ctx, ct.ID); err == nil {
			if detail.State.Health != nil {
				svc.Health = detail.State.Health.Status
			} else {
				svc.Health = "none" // No health check configured
			}

			// Calculate uptime from StartedAt
			if detail.State.Running && detail.State.StartedAt != "" {
				if started, err := time.Parse(time.RFC3339Nano, detail.State.StartedAt); err == nil {
					svc.Uptime = time.Since(started).Round(time.Second).String()
				}
			}
		} else {
			svc.Health = "unknown"
		}

		services = append(services, svc)
	}

	return services, nil
}

// Close releases idle connections.
func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

// do makes an HTTP request to the Docker daemon.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.http.Do(req)
}
