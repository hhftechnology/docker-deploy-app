package docker

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"docker-deploy-app/internal/models"
)

// NewtInjector handles injection of Newt service into Docker Compose files
type NewtInjector struct {
	config *models.NewtConfig
}

// NewNewtInjector creates a new Newt injector
func NewNewtInjector(config *models.NewtConfig) *NewtInjector {
	return &NewtInjector{config: config}
}

// ValidationResult represents the result of Newt validation
type ValidationResult struct {
	Valid        bool     `json:"valid"`
	HasNewt      bool     `json:"has_newt"`
	NetworkOK    bool     `json:"network_ok"`
	Issues       []string `json:"issues"`
	Warnings     []string `json:"warnings"`
	Suggestions  []string `json:"suggestions"`
}

// ProcessCompose processes a docker-compose.yml file and injects Newt service if needed
func (ni *NewtInjector) ProcessCompose(composeContent []byte) ([]byte, *ValidationResult, error) {
	var compose DockerCompose
	if err := yaml.Unmarshal(composeContent, &compose); err != nil {
		return nil, nil, fmt.Errorf("failed to parse docker-compose: %w", err)
	}

	// Validate current compose file
	result := ni.ValidateCompose(&compose)

	// Initialize services map if nil
	if compose.Services == nil {
		compose.Services = make(map[string]ComposeService)
	}

	// Check if newt service already exists
	if existingNewt, exists := compose.Services["newt"]; exists {
		result.HasNewt = true
		// Validate existing newt configuration
		if err := ni.validateNewtService(existingNewt); err != nil {
			result.Issues = append(result.Issues, err.Error())
			// Update the existing newt service with correct config
			compose.Services["newt"] = ni.createNewtService()
			result.Suggestions = append(result.Suggestions, "Updated existing newt service with correct configuration")
		}
	} else {
		// Add newt service
		compose.Services["newt"] = ni.createNewtService()
		result.HasNewt = true
		result.Suggestions = append(result.Suggestions, "Added newt service for tunnel connectivity")
	}

	// Ensure network configuration
	if err := ni.ensureNetworkConfiguration(&compose); err != nil {
		result.Issues = append(result.Issues, err.Error())
	} else {
		result.NetworkOK = true
	}

	// Final validation
	result.Valid = len(result.Issues) == 0

	// Marshal back to YAML
	modifiedContent, err := yaml.Marshal(&compose)
	if err != nil {
		return nil, result, fmt.Errorf("failed to marshal docker-compose: %w", err)
	}

	return modifiedContent, result, nil
}

// ValidateCompose validates a docker-compose file for Newt compatibility
func (ni *NewtInjector) ValidateCompose(compose *DockerCompose) *ValidationResult {
	result := &ValidationResult{
		Valid:       true,
		HasNewt:     false,
		NetworkOK:   true,
		Issues:      []string{},
		Warnings:    []string{},
		Suggestions: []string{},
	}

	// Check if services exist
	if compose.Services == nil || len(compose.Services) == 0 {
		result.Issues = append(result.Issues, "No services defined in docker-compose file")
		result.Valid = false
		return result
	}

	// Check for newt service
	if newtService, exists := compose.Services["newt"]; exists {
		result.HasNewt = true
		if err := ni.validateNewtService(newtService); err != nil {
			result.Issues = append(result.Issues, fmt.Sprintf("Newt service configuration error: %s", err.Error()))
			result.Valid = false
		}
	}

	// Check network configuration
	if err := ni.validateNetworkConfiguration(compose); err != nil {
		result.NetworkOK = false
		result.Warnings = append(result.Warnings, err.Error())
	}

	// Check for potential port conflicts
	ports := make(map[string][]string)
	for serviceName, service := range compose.Services {
		for _, port := range service.Ports {
			hostPort := strings.Split(port, ":")[0]
			ports[hostPort] = append(ports[hostPort], serviceName)
		}
	}

	for port, services := range ports {
		if len(services) > 1 {
			result.Warnings = append(result.Warnings, 
				fmt.Sprintf("Port %s is used by multiple services: %s", port, strings.Join(services, ", ")))
		}
	}

	// Suggest improvements
	if !result.HasNewt {
		result.Suggestions = append(result.Suggestions, "Add newt service for remote tunnel access")
	}

	if len(compose.Networks) == 0 {
		result.Suggestions = append(result.Suggestions, "Define custom networks for better service isolation")
	}

	return result
}

// createNewtService creates a properly configured Newt service
func (ni *NewtInjector) createNewtService() ComposeService {
	service := ComposeService{
		Image:         "fosrl/newt:latest",
		ContainerName: "newt",
		Restart:       "unless-stopped",
		Environment:   ni.config.GetEnvironmentVars(),
		Volumes: []string{
			"/var/run/docker.sock:/var/run/docker.sock:ro",
		},
		Networks: []string{"app_network"},
		Labels: map[string]string{
			"app.type":        "tunnel",
			"app.name":        "newt",
			"app.managed":     "true",
			"traefik.enable": "false",
		},
		HealthCheck: &ComposeHealthCheck{
			Test:        []string{"CMD", "test", "-f", "/tmp/healthy"},
			Interval:    "30s",
			Timeout:     "10s",
			Retries:     3,
			StartPeriod: "60s",
		},
	}

	// Add additional configuration if specified
	if ni.config.Image != "" {
		service.Image = ni.config.Image
	}

	return service
}

// ensureNetworkConfiguration ensures proper network configuration
func (ni *NewtInjector) ensureNetworkConfiguration(compose *DockerCompose) error {
	// Create default network if none exists
	if compose.Networks == nil || len(compose.Networks) == 0 {
		compose.Networks = map[string]ComposeNetwork{
			"app_network": {
				Driver: "bridge",
				Labels: map[string]string{
					"app.managed": "true",
				},
			},
		}
	}

	// Ensure all services are connected to at least one network
	for name, service := range compose.Services {
		if len(service.Networks) == 0 {
			service.Networks = []string{"app_network"}
			compose.Services[name] = service
		} else {
			// Check if app_network is in the service's networks
			hasAppNetwork := false
			for _, network := range service.Networks {
				if network == "app_network" {
					hasAppNetwork = true
					break
				}
			}
			if !hasAppNetwork {
				service.Networks = append(service.Networks, "app_network")
				compose.Services[name] = service
			}
		}
	}

	return nil
}

// validateNewtService validates an existing newt service configuration
func (ni *NewtInjector) validateNewtService(service ComposeService) error {
	// Check image
	if service.Image == "" {
		return fmt.Errorf("newt service missing image")
	}

	if !strings.Contains(service.Image, "newt") {
		return fmt.Errorf("newt service using incorrect image: %s", service.Image)
	}

	// Check required environment variables
	requiredEnvs := map[string]bool{
		"PANGOLIN_ENDPOINT": false,
		"NEWT_ID":          false,
		"NEWT_SECRET":      false,
	}

	for _, env := range service.Environment {
		for required := range requiredEnvs {
			if strings.HasPrefix(env, required+"=") {
				requiredEnvs[required] = true
			}
		}
	}

	var missingEnvs []string
	for env, found := range requiredEnvs {
		if !found {
			missingEnvs = append(missingEnvs, env)
		}
	}

	if len(missingEnvs) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missingEnvs, ", "))
	}

	// Check for Docker socket mount
	hasDockerSocket := false
	for _, volume := range service.Volumes {
		if strings.Contains(volume, "/var/run/docker.sock") {
			hasDockerSocket = true
			break
		}
	}

	if !hasDockerSocket {
		return fmt.Errorf("newt service missing Docker socket mount")
	}

	return nil
}

// validateNetworkConfiguration validates network configuration
func (ni *NewtInjector) validateNetworkConfiguration(compose *DockerCompose) error {
	if len(compose.Networks) == 0 {
		return fmt.Errorf("no networks defined - services may not be able to communicate")
	}

	// Check if services are properly networked
	servicesWithoutNetworks := []string{}
	for name, service := range compose.Services {
		if len(service.Networks) == 0 {
			servicesWithoutNetworks = append(servicesWithoutNetworks, name)
		}
	}

	if len(servicesWithoutNetworks) > 0 {
		return fmt.Errorf("services without network configuration: %s", strings.Join(servicesWithoutNetworks, ", "))
	}

	return nil
}

// InjectNewtIntoFile reads, processes, and writes back a docker-compose file
func (ni *NewtInjector) InjectNewtIntoFile(filePath string) (*ValidationResult, error) {
	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file: %w", err)
	}

	// Process the content
	modifiedContent, result, err := ni.ProcessCompose(content)
	if err != nil {
		return result, err
	}

	// Write back the modified content
	if err := os.WriteFile(filePath, modifiedContent, 0644); err != nil {
		return result, fmt.Errorf("failed to write modified compose file: %w", err)
	}

	return result, nil
}

// PreviewNewtInjection shows what changes would be made without applying them
func (ni *NewtInjector) PreviewNewtInjection(composeContent []byte) (map[string]interface{}, error) {
	var compose DockerCompose
	if err := yaml.Unmarshal(composeContent, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse docker-compose: %w", err)
	}

	preview := map[string]interface{}{
		"has_newt_service": false,
		"will_add_newt":    false,
		"will_add_network": false,
		"changes":          []string{},
	}

	// Check if newt service exists
	if _, exists := compose.Services["newt"]; exists {
		preview["has_newt_service"] = true
	} else {
		preview["will_add_newt"] = true
		preview["changes"] = append(preview["changes"].([]string), "Add newt service")
	}

	// Check if networks need to be added
	if len(compose.Networks) == 0 {
		preview["will_add_network"] = true
		preview["changes"] = append(preview["changes"].([]string), "Add app_network")
	}

	// Check which services will be modified for networking
	servicesNeedingNetworks := []string{}
	for name, service := range compose.Services {
		if len(service.Networks) == 0 {
			servicesNeedingNetworks = append(servicesNeedingNetworks, name)
		}
	}

	if len(servicesNeedingNetworks) > 0 {
		preview["changes"] = append(preview["changes"].([]string), 
			fmt.Sprintf("Add network configuration to services: %s", strings.Join(servicesNeedingNetworks, ", ")))
	}

	preview["newt_service"] = ni.createNewtService()

	return preview, nil
}