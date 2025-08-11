package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"docker-deploy-app/internal/models"
)

// Client wraps Docker client with additional functionality
type Client struct {
	cli *client.Client
	ctx context.Context
}

// NewClient creates a new Docker client wrapper
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &Client{
		cli: cli,
		ctx: context.Background(),
	}, nil
}

// Close closes the Docker client connection
func (c *Client) Close() error {
	return c.cli.Close()
}

// GetContainers retrieves all containers
func (c *Client) GetContainers(all bool) ([]types.Container, error) {
	containers, err := c.cli.ContainerList(c.ctx, types.ContainerListOptions{All: all})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	return containers, nil
}

// GetContainersByLabel retrieves containers with specific label
func (c *Client) GetContainersByLabel(label, value string) ([]types.Container, error) {
	filters := map[string][]string{
		"label": {fmt.Sprintf("%s=%s", label, value)},
	}
	
	containers, err := c.cli.ContainerList(c.ctx, types.ContainerListOptions{
		All:     true,
		Filters: filters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers by label: %w", err)
	}
	return containers, nil
}

// GetStackContainers retrieves containers for a specific stack
func (c *Client) GetStackContainers(stackName string) ([]types.Container, error) {
	return c.GetContainersByLabel("com.docker.compose.project", stackName)
}

// GetContainerStats retrieves resource usage statistics for a container
func (c *Client) GetContainerStats(containerID string) (*models.ServiceStats, error) {
	stats, err := c.cli.ContainerStats(c.ctx, containerID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer stats.Body.Close()

	var containerStats types.StatsJSON
	if err := json.NewDecoder(stats.Body).Decode(&containerStats); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	// Calculate CPU usage percentage
	cpuUsage := calculateCPUUsage(&containerStats)

	// Get memory usage
	memoryUsage := containerStats.MemoryStats.Usage
	memoryLimit := containerStats.MemoryStats.Limit

	// Get network stats
	var networkRx, networkTx int64
	for _, network := range containerStats.Networks {
		networkRx += int64(network.RxBytes)
		networkTx += int64(network.TxBytes)
	}

	// Get block I/O stats
	var blockRead, blockWrite int64
	for _, bioEntry := range containerStats.BloodStats.IoServiceBytesRecursive {
		switch bioEntry.Op {
		case "read":
			blockRead += int64(bioEntry.Value)
		case "write":
			blockWrite += int64(bioEntry.Value)
		}
	}

	return &models.ServiceStats{
		CPUUsage:    cpuUsage,
		MemoryUsage: int64(memoryUsage),
		MemoryLimit: int64(memoryLimit),
		NetworkRx:   networkRx,
		NetworkTx:   networkTx,
		BlockRead:   blockRead,
		BlockWrite:  blockWrite,
		PIDs:        int(containerStats.PidsStats.Current),
		UpdatedAt:   time.Now(),
	}, nil
}

// GetContainerLogs retrieves logs from a container
func (c *Client) GetContainerLogs(containerID string, tail string) (io.ReadCloser, error) {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Tail:       tail,
	}

	logs, err := c.cli.ContainerLogs(c.ctx, containerID, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %w", err)
	}

	return logs, nil
}

// StartContainer starts a container
func (c *Client) StartContainer(containerID string) error {
	err := c.cli.ContainerStart(c.ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	return nil
}

// StopContainer stops a container
func (c *Client) StopContainer(containerID string, timeout *int) error {
	var timeoutSeconds *int
	if timeout != nil {
		timeoutSeconds = timeout
	}

	err := c.cli.ContainerStop(c.ctx, containerID, container.StopOptions{Timeout: timeoutSeconds})
	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	return nil
}

// RestartContainer restarts a container
func (c *Client) RestartContainer(containerID string, timeout *int) error {
	var timeoutSeconds *int
	if timeout != nil {
		timeoutSeconds = timeout
	}

	err := c.cli.ContainerRestart(c.ctx, containerID, container.StopOptions{Timeout: timeoutSeconds})
	if err != nil {
		return fmt.Errorf("failed to restart container: %w", err)
	}
	return nil
}

// GetNetworks retrieves all networks
func (c *Client) GetNetworks() ([]types.NetworkResource, error) {
	networks, err := c.cli.NetworkList(c.ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}
	return networks, nil
}

// GetVolumes retrieves all volumes
func (c *Client) GetVolumes() ([]*types.Volume, error) {
	volumes, err := c.cli.VolumeList(c.ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}
	return volumes.Volumes, nil
}

// GetStackNetworks retrieves networks for a specific stack
func (c *Client) GetStackNetworks(stackName string) ([]types.NetworkResource, error) {
	networks, err := c.GetNetworks()
	if err != nil {
		return nil, err
	}

	var stackNetworks []types.NetworkResource
	for _, network := range networks {
		if project, exists := network.Labels["com.docker.compose.project"]; exists && project == stackName {
			stackNetworks = append(stackNetworks, network)
		}
	}

	return stackNetworks, nil
}

// GetStackVolumes retrieves volumes for a specific stack
func (c *Client) GetStackVolumes(stackName string) ([]*types.Volume, error) {
	volumes, err := c.GetVolumes()
	if err != nil {
		return nil, err
	}

	var stackVolumes []*types.Volume
	for _, volume := range volumes {
		if project, exists := volume.Labels["com.docker.compose.project"]; exists && project == stackName {
			stackVolumes = append(stackVolumes, volume)
		}
	}

	return stackVolumes, nil
}

// InspectContainer gets detailed information about a container
func (c *Client) InspectContainer(containerID string) (types.ContainerJSON, error) {
	container, err := c.cli.ContainerInspect(c.ctx, containerID)
	if err != nil {
		return types.ContainerJSON{}, fmt.Errorf("failed to inspect container: %w", err)
	}
	return container, nil
}

// IsContainerRunning checks if a container is running
func (c *Client) IsContainerRunning(containerID string) (bool, error) {
	container, err := c.InspectContainer(containerID)
	if err != nil {
		return false, err
	}
	return container.State.Running, nil
}

// GetContainerHealth returns the health status of a container
func (c *Client) GetContainerHealth(containerID string) (string, error) {
	container, err := c.InspectContainer(containerID)
	if err != nil {
		return "", err
	}
	
	if container.State.Health != nil {
		return container.State.Health.Status, nil
	}
	
	// If no health check is configured, return status based on running state
	if container.State.Running {
		return "healthy", nil
	}
	return "unhealthy", nil
}

// PullImage pulls a Docker image
func (c *Client) PullImage(imageName string) error {
	reader, err := c.cli.ImagePull(c.ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()

	// Read the response to ensure the pull completes
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to read pull response: %w", err)
	}

	return nil
}

// RemoveContainer removes a container
func (c *Client) RemoveContainer(containerID string, force bool) error {
	err := c.cli.ContainerRemove(c.ctx, containerID, types.ContainerRemoveOptions{Force: force})
	if err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	return nil
}

// calculateCPUUsage calculates CPU usage percentage from stats
func calculateCPUUsage(stats *types.StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)
	
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		return (cpuDelta / systemDelta) * float64(len(stats.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}
	return 0.0
}