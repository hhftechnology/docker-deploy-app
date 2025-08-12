package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"docker-deploy-app/internal/models"
)

// Monitor watches Docker events and container status
type Monitor struct {
	client      *client.Client
	ctx         context.Context
	cancel      context.CancelFunc
	subscribers map[string][]chan *MonitorEvent
	mu          sync.RWMutex
}

// MonitorEvent represents a Docker monitoring event
type MonitorEvent struct {
	Type        string                 `json:"type"`
	Action      string                 `json:"action"`
	ContainerID string                 `json:"container_id"`
	ImageName   string                 `json:"image_name"`
	StackName   string                 `json:"stack_name"`
	ServiceName string                 `json:"service_name"`
	Status      string                 `json:"status"`
	Timestamp   time.Time              `json:"timestamp"`
	Attributes  map[string]interface{} `json:"attributes"`
}

// NewMonitor creates a new Docker monitor
func NewMonitor(dockerClient *client.Client) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Monitor{
		client:      dockerClient,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[string][]chan *MonitorEvent),
	}
}

// Start begins monitoring Docker events
func (m *Monitor) Start() error {
	log.Println("Starting Docker monitor...")

	// Start event monitoring goroutine
	go m.monitorEvents()

	// Start periodic status updates
	go m.periodicStatusUpdate()

	return nil
}

// Stop stops the Docker monitor
func (m *Monitor) Stop() {
	log.Println("Stopping Docker monitor...")
	m.cancel()
}

// Subscribe subscribes to monitor events for a specific stack
func (m *Monitor) Subscribe(stackName string) chan *MonitorEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan *MonitorEvent, 100)
	
	if m.subscribers[stackName] == nil {
		m.subscribers[stackName] = []chan *MonitorEvent{}
	}
	
	m.subscribers[stackName] = append(m.subscribers[stackName], ch)
	return ch
}

// Unsubscribe removes a subscription
func (m *Monitor) Unsubscribe(stackName string, ch chan *MonitorEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if subscribers, exists := m.subscribers[stackName]; exists {
		for i, subscriber := range subscribers {
			if subscriber == ch {
				close(ch)
				m.subscribers[stackName] = append(subscribers[:i], subscribers[i+1:]...)
				break
			}
		}
	}
}

// monitorEvents listens for Docker events
func (m *Monitor) monitorEvents() {
	eventsCh, errCh := m.client.Events(m.ctx, types.EventsOptions{})

	for {
		select {
		case event := <-eventsCh:
			m.handleDockerEvent(event)
		case err := <-errCh:
			if err != nil {
				log.Printf("Docker events error: %v", err)
				time.Sleep(5 * time.Second) // Reconnect delay
			}
		case <-m.ctx.Done():
			return
		}
	}
}

// handleDockerEvent processes a Docker event
func (m *Monitor) handleDockerEvent(event events.Message) {
	if event.Type != events.ContainerEventType {
		return // Only handle container events
	}

	containerID := event.Actor.ID
	if len(containerID) > 12 {
		containerID = containerID[:12] // Short ID
	}

	// Get container info
	container, err := m.client.ContainerInspect(m.ctx, event.Actor.ID)
	if err != nil {
		return
	}

	// Extract stack and service information from labels
	stackName := m.getStackName(container.Config.Labels)
	serviceName := m.getServiceName(container.Config.Labels)

	if stackName == "" {
		return // Not a compose stack
	}

	monitorEvent := &MonitorEvent{
		Type:        "container",
		Action:      event.Action,
		ContainerID: containerID,
		ImageName:   container.Config.Image,
		StackName:   stackName,
		ServiceName: serviceName,
		Status:      container.State.Status,
		Timestamp:   time.Unix(event.Time, 0),
		Attributes: map[string]interface{}{
			"labels": container.Config.Labels,
		},
	}

	m.publishEvent(stackName, monitorEvent)
}

// periodicStatusUpdate sends periodic status updates
func (m *Monitor) periodicStatusUpdate() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.sendStatusUpdates()
		case <-m.ctx.Done():
			return
		}
	}
}

// sendStatusUpdates sends status updates for all monitored stacks
func (m *Monitor) sendStatusUpdates() {
	m.mu.RLock()
	stackNames := make([]string, 0, len(m.subscribers))
	for stackName := range m.subscribers {
		stackNames = append(stackNames, stackName)
	}
	m.mu.RUnlock()

	for _, stackName := range stackNames {
		stats := m.getStackStats(stackName)
		if stats != nil {
			event := &MonitorEvent{
				Type:      "status_update",
				StackName: stackName,
				Timestamp: time.Now(),
				Attributes: map[string]interface{}{
					"stats": stats,
				},
			}
			m.publishEvent(stackName, event)
		}
	}
}

// getStackStats gets current statistics for a stack
func (m *Monitor) getStackStats(stackName string) *models.StackStats {
	containers, err := m.client.ContainerList(m.ctx, types.ContainerListOptions{
		All: true,
	})

	if err != nil {
		return nil
	}

	// Filter containers by stack name
	var stackContainers []types.Container
	for _, container := range containers {
		if m.getStackName(container.Labels) == stackName {
			stackContainers = append(stackContainers, container)
		}
	}

	var totalCPU float64
	var totalMemory, totalMemoryLimit int64
	var totalNetworkRx, totalNetworkTx int64

	for _, container := range stackContainers {
		if container.State != "running" {
			continue
		}

		// Get container stats
		stats, err := m.client.ContainerStats(m.ctx, container.ID, false)
		if err != nil {
			continue
		}

		var containerStats types.StatsJSON
		if err := json.NewDecoder(stats.Body).Decode(&containerStats); err != nil {
			stats.Body.Close()
			continue
		}
		stats.Body.Close()

		// Aggregate stats
		totalCPU += calculateCPUUsage(&containerStats)
		totalMemory += int64(containerStats.MemoryStats.Usage)
		totalMemoryLimit += int64(containerStats.MemoryStats.Limit)

		for _, network := range containerStats.Networks {
			totalNetworkRx += int64(network.RxBytes)
			totalNetworkTx += int64(network.TxBytes)
		}
	}

	return &models.StackStats{
		CPUUsage:    totalCPU,
		MemoryUsage: totalMemory,
		MemoryLimit: totalMemoryLimit,
		NetworkRx:   totalNetworkRx,
		NetworkTx:   totalNetworkTx,
		UpdatedAt:   time.Now(),
	}
}

// publishEvent sends an event to all subscribers of a stack
func (m *Monitor) publishEvent(stackName string, event *MonitorEvent) {
	m.mu.RLock()
	subscribers := m.subscribers[stackName]
	m.mu.RUnlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- event:
		default:
			// Channel is full, skip this subscriber
		}
	}
}

// getStackName extracts stack name from container labels
func (m *Monitor) getStackName(labels map[string]string) string {
	// Try different label keys for stack name
	stackKeys := []string{
		"com.docker.compose.project",
		"docker-compose.project",
		"compose.project",
	}

	for _, key := range stackKeys {
		if value, exists := labels[key]; exists {
			return value
		}
	}

	return ""
}

// getServiceName extracts service name from container labels
func (m *Monitor) getServiceName(labels map[string]string) string {
	// Try different label keys for service name
	serviceKeys := []string{
		"com.docker.compose.service",
		"docker-compose.service",
		"compose.service",
	}

	for _, key := range serviceKeys {
		if value, exists := labels[key]; exists {
			return value
		}
	}

	return ""
}

// GetContainerStatus returns current status of containers in a stack
func (m *Monitor) GetContainerStatus(stackName string) ([]models.StackService, error) {
	containers, err := m.client.ContainerList(m.ctx, types.ContainerListOptions{
		All: true,
	})

	if err != nil {
		return nil, err
	}

	var services []models.StackService
	for _, container := range containers {
		if m.getStackName(container.Labels) != stackName {
			continue
		}
		
		serviceName := m.getServiceName(container.Labels)
		
		service := models.StackService{
			Name:      serviceName,
			Image:     container.Image,
			Status:    container.Status,
			State:     container.State,
			CreatedAt: time.Unix(container.Created, 0),
			Labels:    container.Labels,
		}

		// Convert ports
		for _, port := range container.Ports {
			servicePort := models.ServicePort{
				HostPort:      int(port.PublicPort),
				ContainerPort: int(port.PrivatePort),
				Protocol:      port.Type,
				HostIP:        port.IP,
			}
			service.Ports = append(service.Ports, servicePort)
		}

		// Get environment variables from detailed container info
		containerInfo, err := m.client.ContainerInspect(m.ctx, container.ID)
		if err == nil {
			service.Environment = make(map[string]string)
			for _, env := range containerInfo.Config.Env {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) == 2 {
					service.Environment[parts[0]] = parts[1]
				}
			}
			
			if containerInfo.State.Health != nil {
				service.Health = containerInfo.State.Health.Status
			} else if containerInfo.State.Running {
				service.Health = "healthy"
			} else {
				service.Health = "unhealthy"
			}
		} else {
			service.Health = "unknown"
		}

		services = append(services, service)
	}

	return services, nil

		services = append(services, service)
	}

	return services, nil
}

// IsStackHealthy checks if all containers in a stack are healthy
func (m *Monitor) IsStackHealthy(stackName string) (bool, error) {
	services, err := m.GetContainerStatus(stackName)
	if err != nil {
		return false, err
	}

	for _, service := range services {
		if service.State != "running" {
			return false, nil
		}
		if service.Health == "unhealthy" {
			return false, nil
		}
	}

	return len(services) > 0, nil
}

// calculateCPUUsage calculates CPU usage percentage from container stats
func calculateCPUUsage(stats *types.StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)
	
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		return (cpuDelta / systemDelta) * float64(len(stats.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}
	return 0.0
}