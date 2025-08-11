package models

import (
	"time"
)

// NewtConfig represents a Newt tunnel configuration
type NewtConfig struct {
	ID        int       `json:"id" db:"id"`
	Endpoint  string    `json:"endpoint" db:"endpoint"`
	NewtID    string    `json:"newt_id" db:"newt_id"`
	Secret    string    `json:"newt_secret" db:"newt_secret"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// NewtStatus represents the current status of a Newt tunnel
type NewtStatus struct {
	ServiceName   string    `json:"service_name"`
	ContainerID   string    `json:"container_id"`
	Status        string    `json:"status"`
	Health        string    `json:"health"`
	TunnelActive  bool      `json:"tunnel_active"`
	TunnelURL     string    `json:"tunnel_url"`
	ConnectedAt   *time.Time `json:"connected_at"`
	LastPing      *time.Time `json:"last_ping"`
	BytesIn       int64     `json:"bytes_in"`
	BytesOut      int64     `json:"bytes_out"`
	ErrorCount    int       `json:"error_count"`
	LastError     string    `json:"last_error"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NewtValidationResult represents the result of Newt configuration validation
type NewtValidationResult struct {
	Valid         bool     `json:"valid"`
	Reachable     bool     `json:"reachable"`
	Authenticated bool     `json:"authenticated"`
	Issues        []string `json:"issues"`
	Version       string   `json:"version"`
	Features      []string `json:"features"`
	TestedAt      time.Time `json:"tested_at"`
}

// NewtServiceConfig represents Newt service configuration for Docker Compose
type NewtServiceConfig struct {
	Image         string            `json:"image"`
	ContainerName string            `json:"container_name"`
	Restart       string            `json:"restart"`
	Environment   []string          `json:"environment"`
	Volumes       []string          `json:"volumes"`
	Networks      []string          `json:"networks"`
	HealthCheck   *NewtHealthCheck  `json:"healthcheck,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	DependsOn     []string          `json:"depends_on,omitempty"`
}

// NewtHealthCheck represents health check configuration for Newt service
type NewtHealthCheck struct {
	Test     []string `json:"test"`
	Interval string   `json:"interval"`
	Timeout  string   `json:"timeout"`
	Retries  int      `json:"retries"`
	StartPeriod string `json:"start_period,omitempty"`
}

// NewtTunnelInfo contains information about an active tunnel
type NewtTunnelInfo struct {
	ServiceName string            `json:"service_name"`
	LocalPort   int               `json:"local_port"`
	TunnelURL   string            `json:"tunnel_url"`
	Protocol    string            `json:"protocol"`
	Status      string            `json:"status"`
	Metadata    map[string]string `json:"metadata"`
}

// NewtConnectionTest represents a connection test result
type NewtConnectionTest struct {
	TestType    string    `json:"test_type"`
	Success     bool      `json:"success"`
	ResponseTime time.Duration `json:"response_time"`
	Message     string    `json:"message"`
	TestedAt    time.Time `json:"tested_at"`
}

// Validate validates Newt configuration
func (nc *NewtConfig) Validate() error {
	if nc.Endpoint == "" {
		return ErrNewtEndpointRequired
	}
	if nc.NewtID == "" {
		return ErrNewtIDRequired
	}
	if nc.Secret == "" {
		return ErrNewtSecretRequired
	}
	return nil
}

// IsConfigured returns true if all required fields are set
func (nc *NewtConfig) IsConfigured() bool {
	return nc.Endpoint != "" && nc.NewtID != "" && nc.Secret != ""
}

// GetEnvironmentVars returns environment variables for Docker Compose
func (nc *NewtConfig) GetEnvironmentVars() []string {
	return []string{
		"PANGOLIN_ENDPOINT=" + nc.Endpoint,
		"NEWT_ID=" + nc.NewtID,
		"NEWT_SECRET=" + nc.Secret,
		"LOG_LEVEL=INFO",
		"HEALTH_FILE=/tmp/healthy",
	}
}

// IsHealthy returns true if Newt service is healthy
func (ns *NewtStatus) IsHealthy() bool {
	return ns.Health == "healthy" && ns.TunnelActive
}

// IsConnected returns true if tunnel is connected
func (ns *NewtStatus) IsConnected() bool {
	return ns.TunnelActive && ns.ConnectedAt != nil
}

// GetUptime returns tunnel uptime duration
func (ns *NewtStatus) GetUptime() time.Duration {
	if ns.ConnectedAt == nil {
		return 0
	}
	return time.Since(*ns.ConnectedAt)
}

// HasErrors returns true if there are recent errors
func (ns *NewtStatus) HasErrors() bool {
	return ns.ErrorCount > 0
}

// GetDataTransferred returns total bytes transferred
func (ns *NewtStatus) GetDataTransferred() int64 {
	return ns.BytesIn + ns.BytesOut
}

// IsValid returns true if validation was successful
func (nvr *NewtValidationResult) IsValid() bool {
	return nvr.Valid && nvr.Reachable && nvr.Authenticated
}

// HasIssues returns true if there are validation issues
func (nvr *NewtValidationResult) HasIssues() bool {
	return len(nvr.Issues) > 0
}

// GetDefaultServiceConfig returns default Newt service configuration
func GetDefaultServiceConfig() *NewtServiceConfig {
	return &NewtServiceConfig{
		Image:         "fosrl/newt:latest",
		ContainerName: "newt",
		Restart:       "unless-stopped",
		Volumes: []string{
			"/var/run/docker.sock:/var/run/docker.sock:ro",
		},
		Networks: []string{"app_network"},
		HealthCheck: &NewtHealthCheck{
			Test:     []string{"CMD", "test", "-f", "/tmp/healthy"},
			Interval: "30s",
			Timeout:  "10s",
			Retries:  3,
			StartPeriod: "60s",
		},
		Labels: map[string]string{
			"app.type": "tunnel",
			"app.name": "newt",
		},
	}
}

// SetEnvironment sets environment variables for the service
func (nsc *NewtServiceConfig) SetEnvironment(config *NewtConfig) {
	nsc.Environment = config.GetEnvironmentVars()
}

// AddNetwork adds a network to the service configuration
func (nsc *NewtServiceConfig) AddNetwork(network string) {
	for _, net := range nsc.Networks {
		if net == network {
			return // Already exists
		}
	}
	nsc.Networks = append(nsc.Networks, network)
}

// IsActive returns true if tunnel is active
func (nti *NewtTunnelInfo) IsActive() bool {
	return nti.Status == "active" || nti.Status == "connected"
}

// GetFullURL returns the complete tunnel URL with protocol
func (nti *NewtTunnelInfo) GetFullURL() string {
	if nti.Protocol != "" && nti.TunnelURL != "" {
		return nti.Protocol + "://" + nti.TunnelURL
	}
	return nti.TunnelURL
}

// IsSuccessful returns true if connection test was successful
func (nct *NewtConnectionTest) IsSuccessful() bool {
	return nct.Success
}

// GetFormattedResponseTime returns formatted response time string
func (nct *NewtConnectionTest) GetFormattedResponseTime() string {
	if nct.ResponseTime < time.Millisecond {
		return nct.ResponseTime.String()
	}
	return nct.ResponseTime.Truncate(time.Millisecond).String()
}