package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"docker-deploy-app/internal/models"
)

// ComposeManager handles Docker Compose operations
type ComposeManager struct {
	workDir string
	timeout time.Duration
}

// NewComposeManager creates a new compose manager
func NewComposeManager(workDir string, timeout time.Duration) *ComposeManager {
	return &ComposeManager{
		workDir: workDir,
		timeout: timeout,
	}
}

// DockerCompose represents a docker-compose.yml structure
type DockerCompose struct {
	Version  string                    `yaml:"version"`
	Services map[string]ComposeService `yaml:"services"`
	Networks map[string]ComposeNetwork `yaml:"networks,omitempty"`
	Volumes  map[string]ComposeVolume  `yaml:"volumes,omitempty"`
}

// ComposeService represents a service in docker-compose
type ComposeService struct {
	Image         string            `yaml:"image"`
	ContainerName string            `yaml:"container_name,omitempty"`
	Restart       string            `yaml:"restart,omitempty"`
	Environment   []string          `yaml:"environment,omitempty"`
	Ports         []string          `yaml:"ports,omitempty"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	Networks      []string          `yaml:"networks,omitempty"`
	DependsOn     []string          `yaml:"depends_on,omitempty"`
	HealthCheck   *ComposeHealthCheck `yaml:"healthcheck,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
	Command       interface{}       `yaml:"command,omitempty"`
	Entrypoint    interface{}       `yaml:"entrypoint,omitempty"`
}

// ComposeHealthCheck represents health check configuration
type ComposeHealthCheck struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
}

// ComposeNetwork represents a network in docker-compose
type ComposeNetwork struct {
	Driver   string            `yaml:"driver,omitempty"`
	External bool              `yaml:"external,omitempty"`
	Name     string            `yaml:"name,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}

// ComposeVolume represents a volume in docker-compose
type ComposeVolume struct {
	Driver   string            `yaml:"driver,omitempty"`
	External bool              `yaml:"external,omitempty"`
	Name     string            `yaml:"name,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}

// DeployOptions holds options for deployment
type DeployOptions struct {
	StackName   string
	ProjectDir  string
	EnvVars     map[string]string
	BuildArgs   map[string]string
	Detached    bool
	PullImages  bool
}

// Deploy deploys a Docker Compose stack
func (cm *ComposeManager) Deploy(options DeployOptions) error {
	// Create project directory
	projectDir := filepath.Join(cm.workDir, options.StackName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Change to project directory
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(projectDir); err != nil {
		return fmt.Errorf("failed to change to project directory: %w", err)
	}

	// Copy compose files if source directory is specified
	if options.ProjectDir != "" {
		if err := cm.copyComposeFiles(options.ProjectDir, projectDir); err != nil {
			return fmt.Errorf("failed to copy compose files: %w", err)
		}
	}

	// Create .env file if environment variables are provided
	if len(options.EnvVars) > 0 {
		if err := cm.createEnvFile(projectDir, options.EnvVars); err != nil {
			return fmt.Errorf("failed to create .env file: %w", err)
		}
	}

	// Build command
	args := []string{"compose"}
	
	// Add project name
	args = append(args, "--project-name", options.StackName)

	// Pull images if requested
	if options.PullImages {
		pullArgs := append(args, "pull")
		if err := cm.runCommand("docker", pullArgs); err != nil {
			return fmt.Errorf("failed to pull images: %w", err)
		}
	}

	// Deploy
	args = append(args, "up")
	if options.Detached {
		args = append(args, "--detach")
	}

	return cm.runCommand("docker", args)
}

// Stop stops a Docker Compose stack
func (cm *ComposeManager) Stop(stackName string) error {
	args := []string{"compose", "--project-name", stackName, "stop"}
	return cm.runCommand("docker", args)
}

// Start starts a Docker Compose stack
func (cm *ComposeManager) Start(stackName string) error {
	args := []string{"compose", "--project-name", stackName, "start"}
	return cm.runCommand("docker", args)
}

// Restart restarts a Docker Compose stack
func (cm *ComposeManager) Restart(stackName string) error {
	args := []string{"compose", "--project-name", stackName, "restart"}
	return cm.runCommand("docker", args)
}

// Down removes a Docker Compose stack
func (cm *ComposeManager) Down(stackName string, removeVolumes bool) error {
	args := []string{"compose", "--project-name", stackName, "down"}
	if removeVolumes {
		args = append(args, "--volumes")
	}
	return cm.runCommand("docker", args)
}

// Logs retrieves logs from a Docker Compose stack
func (cm *ComposeManager) Logs(stackName string, follow bool, tail int) (*exec.Cmd, error) {
	args := []string{"compose", "--project-name", stackName, "logs"}
	if follow {
		args = append(args, "--follow")
	}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}

	cmd := exec.Command("docker", args...)
	return cmd, nil
}

// GetServices retrieves services from a stack
func (cm *ComposeManager) GetServices(stackName string) ([]models.StackService, error) {
	args := []string{"compose", "--project-name", stackName, "ps", "--format", "json"}
	
	cmd := exec.Command("docker", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %w", err)
	}

	// Parse JSON output and convert to StackService
	var services []models.StackService
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		
		var service models.StackService
		if err := json.Unmarshal([]byte(line), &service); err != nil {
			continue // Skip invalid JSON lines
		}
		services = append(services, service)
	}

	return services, nil
}

// ParseComposeFile parses a docker-compose.yml file
func (cm *ComposeManager) ParseComposeFile(filePath string) (*DockerCompose, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file: %w", err)
	}

	var compose DockerCompose
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}

	return &compose, nil
}

// WriteComposeFile writes a docker-compose.yml file
func (cm *ComposeManager) WriteComposeFile(filePath string, compose *DockerCompose) error {
	data, err := yaml.Marshal(compose)
	if err != nil {
		return fmt.Errorf("failed to marshal compose data: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %w", err)
	}

	return nil
}

// ValidateCompose validates a docker-compose configuration
func (cm *ComposeManager) ValidateCompose(stackName string) error {
	args := []string{"compose", "--project-name", stackName, "config", "--quiet"}
	return cm.runCommand("docker", args)
}

// GetStackStatus returns the status of a stack
func (cm *ComposeManager) GetStackStatus(stackName string) (models.StackStatus, error) {
	services, err := cm.GetServices(stackName)
	if err != nil {
		return models.StackStatusUnknown, err
	}

	if len(services) == 0 {
		return models.StackStatusStopped, nil
	}

	runningCount := 0
	for _, service := range services {
		if service.Status == "running" {
			runningCount++
		}
	}

	if runningCount == len(services) {
		return models.StackStatusRunning, nil
	} else if runningCount == 0 {
		return models.StackStatusStopped, nil
	} else {
		return models.StackStatusPartial, nil
	}
}

// copyComposeFiles copies compose files from source to destination
func (cm *ComposeManager) copyComposeFiles(srcDir, destDir string) error {
	composeFiles := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"docker-compose.override.yml",
		"docker-compose.override.yaml",
	}

	for _, file := range composeFiles {
		srcPath := filepath.Join(srcDir, file)
		destPath := filepath.Join(destDir, file)

		if _, err := os.Stat(srcPath); err == nil {
			if err := copyFile(srcPath, destPath); err != nil {
				return fmt.Errorf("failed to copy %s: %w", file, err)
			}
		}
	}

	return nil
}

// createEnvFile creates a .env file with environment variables
func (cm *ComposeManager) createEnvFile(dir string, envVars map[string]string) error {
	envPath := filepath.Join(dir, ".env")
	
	var lines []string
	for key, value := range envVars {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	content := strings.Join(lines, "\n")
	return os.WriteFile(envPath, []byte(content), 0644)
}

// runCommand executes a command with timeout
func (cm *ComposeManager) runCommand(command string, args []string) error {
	cmd := exec.Command(command, args...)
	
	if cm.timeout > 0 {
		go func() {
			time.Sleep(cm.timeout)
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
		}()
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %s %v: %w", command, args, err)
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}