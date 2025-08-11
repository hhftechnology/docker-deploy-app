package models

import (
	"time"
)

// StackStatus represents the current status of a stack
type StackStatus string

const (
	StackStatusRunning StackStatus = "running"
	StackStatusStopped StackStatus = "stopped"
	StackStatusPartial StackStatus = "partial"
	StackStatusUnknown StackStatus = "unknown"
)

// Stack represents a Docker Compose stack
type Stack struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Status       StackStatus         `json:"status"`
	Services     []StackService      `json:"services"`
	Networks     []StackNetwork      `json:"networks"`
	Volumes      []StackVolume       `json:"volumes"`
	DeploymentID string              `json:"deployment_id"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Stats        *StackStats         `json:"stats,omitempty"`
}

// StackService represents a service within a stack
type StackService struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Status      string            `json:"status"`
	State       string            `json:"state"`
	Health      string            `json:"health"`
	Ports       []ServicePort     `json:"ports"`
	Environment map[string]string `json:"environment"`
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time         `json:"created_at"`
	Stats       *ServiceStats     `json:"stats,omitempty"`
}

// ServicePort represents a port mapping for a service
type ServicePort struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"host_ip"`
}

// StackNetwork represents a network within a stack
type StackNetwork struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver"`
	Scope    string            `json:"scope"`
	Internal bool              `json:"internal"`
	Labels   map[string]string `json:"labels"`
}

// StackVolume represents a volume within a stack
type StackVolume struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	MountPoint string            `json:"mount_point"`
	Size       int64             `json:"size"`
	Labels     map[string]string `json:"labels"`
}

// StackStats represents resource usage statistics for a stack
type StackStats struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage int64   `json:"memory_usage"`
	MemoryLimit int64   `json:"memory_limit"`
	NetworkRx   int64   `json:"network_rx"`
	NetworkTx   int64   `json:"network_tx"`
	BlockRead   int64   `json:"block_read"`
	BlockWrite  int64   `json:"block_write"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ServiceStats represents resource usage statistics for a service
type ServiceStats struct {
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  int64   `json:"memory_usage"`
	MemoryLimit  int64   `json:"memory_limit"`
	NetworkRx    int64   `json:"network_rx"`
	NetworkTx    int64   `json:"network_tx"`
	BlockRead    int64   `json:"block_read"`
	BlockWrite   int64   `json:"block_write"`
	PIDs         int     `json:"pids"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// StackOperation represents an operation that can be performed on a stack
type StackOperation string

const (
	OperationStart   StackOperation = "start"
	OperationStop    StackOperation = "stop"
	OperationRestart StackOperation = "restart"
	OperationPause   StackOperation = "pause"
	OperationUnpause StackOperation = "unpause"
	OperationRemove  StackOperation = "remove"
)

// StackLogEntry represents a log entry from stack services
type StackLogEntry struct {
	ServiceName string    `json:"service_name"`
	Timestamp   time.Time `json:"timestamp"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	Stream      string    `json:"stream"` // stdout, stderr
}

// IsRunning returns true if all services in the stack are running
func (s *Stack) IsRunning() bool {
	if len(s.Services) == 0 {
		return false
	}
	for _, service := range s.Services {
		if service.Status != "running" {
			return false
		}
	}
	return true
}

// GetRunningServices returns the count of running services
func (s *Stack) GetRunningServices() int {
	count := 0
	for _, service := range s.Services {
		if service.Status == "running" {
			count++
		}
	}
	return count
}

// GetTotalServices returns the total number of services
func (s *Stack) GetTotalServices() int {
	return len(s.Services)
}

// GetService returns a service by name
func (s *Stack) GetService(name string) (*StackService, bool) {
	for i := range s.Services {
		if s.Services[i].Name == name {
			return &s.Services[i], true
		}
	}
	return nil, false
}

// HasNewtService returns true if stack contains a newt service
func (s *Stack) HasNewtService() bool {
	_, exists := s.GetService("newt")
	return exists
}

// GetNewtTunnelStatus returns the status of the newt tunnel service
func (s *Stack) GetNewtTunnelStatus() string {
	if service, exists := s.GetService("newt"); exists {
		return service.Health
	}
	return "unavailable"
}

// CanPerformOperation returns true if the operation can be performed on the stack
func (s *Stack) CanPerformOperation(op StackOperation) bool {
	switch op {
	case OperationStart:
		return s.Status == StackStatusStopped
	case OperationStop:
		return s.Status == StackStatusRunning || s.Status == StackStatusPartial
	case OperationRestart:
		return s.Status == StackStatusRunning || s.Status == StackStatusPartial
	case OperationRemove:
		return s.Status == StackStatusStopped
	default:
		return false
	}
}

// UpdateStatus updates the stack status based on service states
func (s *Stack) UpdateStatus() {
	if len(s.Services) == 0 {
		s.Status = StackStatusUnknown
		return
	}

	runningCount := s.GetRunningServices()
	totalCount := s.GetTotalServices()

	if runningCount == totalCount {
		s.Status = StackStatusRunning
	} else if runningCount == 0 {
		s.Status = StackStatusStopped
	} else {
		s.Status = StackStatusPartial
	}

	s.UpdatedAt = time.Now()
}

// GetMemoryUsagePercentage returns memory usage as percentage
func (ss *ServiceStats) GetMemoryUsagePercentage() float64 {
	if ss.MemoryLimit == 0 {
		return 0
	}
	return float64(ss.MemoryUsage) / float64(ss.MemoryLimit) * 100
}

// FormatMemoryUsage returns formatted memory usage string
func (ss *ServiceStats) FormatMemoryUsage() string {
	return formatBytes(ss.MemoryUsage)
}

// FormatMemoryLimit returns formatted memory limit string
func (ss *ServiceStats) FormatMemoryLimit() string {
	return formatBytes(ss.MemoryLimit)
}

// Helper function to format bytes
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}