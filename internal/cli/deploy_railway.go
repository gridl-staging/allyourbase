// Package cli deploy_railway implements the Railway deployment provider, managing project and service creation, environment configuration, and deployment orchestration through the Railway GraphQL API.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	railDefaultBaseURL     = "https://backboard.railway.app/graphql/v2"
	railDefaultServiceName = "ayb"
	railDefaultWaitTimeout = 2 * time.Minute
	railPollInterval       = 2 * time.Second

	railOptionProjectName = "project_name"
	railOptionServiceName = "service_name"
	railOptionImage       = "image"
)

// RailwayClient defines the interface for interacting with the Railway API.
type RailwayClient interface {
	CreateProject(ctx context.Context, name string) (RailwayProject, error)
	GetOrCreateProject(ctx context.Context, name string) (RailwayProject, error)
	GetOrCreateService(ctx context.Context, projectID, name string) (RailwayService, error)
	CreateService(ctx context.Context, projectID string, name string) (RailwayService, error)
	SetConfigVariables(ctx context.Context, projectID string, variables map[string]string) error
	TriggerDeployment(ctx context.Context, projectID, serviceID, image string) (RailwayDeployment, error)
	WaitForDeploymentSuccess(ctx context.Context, projectID, deploymentID string) (RailwayDeployment, error)
}

// railwayHTTPClient implements RailwayClient with GraphQL API calls.
type railwayHTTPClient struct {
	client  *http.Client
	token   string
	baseURL string
}

func newRailwayHTTPClient(token string) *railwayHTTPClient {
	return &railwayHTTPClient{
		client:  &http.Client{Timeout: 30 * time.Second},
		token:   strings.TrimSpace(token),
		baseURL: railDefaultBaseURL,
	}
}

type railwayGraphQLOperation struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type railwayGraphQLError struct {
	Message string `json:"message"`
}

type railwayGraphQLResponse struct {
	Data   json.RawMessage       `json:"data"`
	Errors []railwayGraphQLError `json:"errors"`
}

// doGraphQL executes a GraphQL query or mutation against the Railway API with the provided variables, handles HTTP errors and GraphQL response errors, and unmarshals the response data into the output parameter.
func (c *railwayHTTPClient) doGraphQL(ctx context.Context, query string, variables map[string]interface{}, out any) error {
	op := railwayGraphQLOperation{Query: query, Variables: variables}
	raw, err := json.Marshal(op)
	if err != nil {
		return fmt.Errorf("marshal railway request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("create railway request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform railway request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read railway response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("railway API error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded railwayGraphQLResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("decode railway response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		msgs := make([]string, 0, len(decoded.Errors))
		for _, e := range decoded.Errors {
			if strings.TrimSpace(e.Message) != "" {
				msgs = append(msgs, strings.TrimSpace(e.Message))
			}
		}
		if len(msgs) == 0 {
			msgs = append(msgs, "unknown GraphQL error")
		}
		return fmt.Errorf("GraphQL error: %s", strings.Join(msgs, "; "))
	}

	if out != nil {
		if len(decoded.Data) == 0 {
			return fmt.Errorf("railway response missing data")
		}
		if err := json.Unmarshal(decoded.Data, out); err != nil {
			return fmt.Errorf("decode railway data: %w", err)
		}
	}
	return nil
}

// RailwayProject represents a Railway project response.
type RailwayProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RailwayService represents a Railway service response.
type RailwayService struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RailwayDeployment represents a Railway deployment response.
type RailwayDeployment struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	URL    string `json:"url"`
}

// CreateProject creates a new Railway project.
func (c *railwayHTTPClient) CreateProject(ctx context.Context, name string) (RailwayProject, error) {
	mutation := `
		mutation($name: String!) {
			projectCreate(name: $name) {
				id
				name
			}
		}
	`

	var data struct {
		ProjectCreate RailwayProject `json:"projectCreate"`
	}
	if err := c.doGraphQL(ctx, mutation, map[string]interface{}{"name": name}, &data); err != nil {
		return RailwayProject{}, err
	}
	return data.ProjectCreate, nil
}

// GetOrCreateProject tries to find an existing project or creates a new one.
func (c *railwayHTTPClient) GetOrCreateProject(ctx context.Context, name string) (RailwayProject, error) {
	query := `
		query {
			projects {
				nodes {
					id
					name
				}
			}
		}
	`

	var data struct {
		Projects struct {
			Nodes []RailwayProject `json:"nodes"`
		} `json:"projects"`
	}
	if err := c.doGraphQL(ctx, query, nil, &data); err != nil {
		return RailwayProject{}, err
	}

	for _, p := range data.Projects.Nodes {
		if p.Name == name {
			return p, nil
		}
	}
	return c.CreateProject(ctx, name)
}

// CreateService creates a new Railway service within a project.
func (c *railwayHTTPClient) CreateService(ctx context.Context, projectID string, name string) (RailwayService, error) {
	mutation := `
		mutation($projectId: String!, $name: String!) {
			serviceCreate(projectId: $projectId, name: $name) {
				id
				name
			}
		}
	`

	var data struct {
		ServiceCreate RailwayService `json:"serviceCreate"`
	}
	if err := c.doGraphQL(ctx, mutation, map[string]interface{}{"projectId": projectID, "name": name}, &data); err != nil {
		return RailwayService{}, err
	}
	return data.ServiceCreate, nil
}

// GetOrCreateService tries to find an existing service by name in the project or creates a new one.
func (c *railwayHTTPClient) GetOrCreateService(ctx context.Context, projectID, name string) (RailwayService, error) {
	query := `
		query($projectId: String!) {
			project(id: $projectId) {
				services {
					nodes {
						id
						name
					}
				}
			}
		}
	`

	var data struct {
		Project struct {
			Services struct {
				Nodes []RailwayService `json:"nodes"`
			} `json:"services"`
		} `json:"project"`
	}
	if err := c.doGraphQL(ctx, query, map[string]interface{}{"projectId": projectID}, &data); err != nil {
		return RailwayService{}, err
	}
	for _, svc := range data.Project.Services.Nodes {
		if svc.Name == name {
			return svc, nil
		}
	}
	return c.CreateService(ctx, projectID, name)
}

// SetConfigVariables sets configuration variables in the project.
func (c *railwayHTTPClient) SetConfigVariables(ctx context.Context, projectID string, variables map[string]string) error {
	mutation := `
		mutation($projectId: String!, $config: [InputConfigKeyValuePair!]!) {
			environmentUpdate(projectId: $projectId, config: $config) {
				id
			}
		}
	`
	configVars := make([]map[string]any, 0, len(variables))
	for key, value := range variables {
		configVars = append(configVars, map[string]any{
			"key":           key,
			"value":         value,
			"isSecret":      true,
			"isHighlighted": false,
			"isBuiltIn":     false,
			"origin":        "FROM_VARIABLE",
		})
	}

	return c.doGraphQL(ctx, mutation, map[string]interface{}{"projectId": projectID, "config": configVars}, &struct {
		EnvironmentUpdate struct {
			ID string `json:"id"`
		} `json:"environmentUpdate"`
	}{})
}

// TriggerDeployment triggers a deployment for a service with a specific image.
func (c *railwayHTTPClient) TriggerDeployment(ctx context.Context, projectID, serviceID, image string) (RailwayDeployment, error) {
	mutation := `
		mutation($projectId: String!, $serviceId: String!, $image: String!) {
			deploymentCreate(
				projectId: $projectId
				serviceId: $serviceId
				imageUrl: $image
				environmentName: "production"
			) {
				id
				status
				url
			}
		}
	`
	var data struct {
		DeploymentCreate RailwayDeployment `json:"deploymentCreate"`
	}
	if err := c.doGraphQL(ctx, mutation, map[string]interface{}{"projectId": projectID, "serviceId": serviceID, "image": image}, &data); err != nil {
		return RailwayDeployment{}, err
	}
	return data.DeploymentCreate, nil
}

// WaitForDeploymentSuccess waits for a deployment to reach success status.
func (c *railwayHTTPClient) WaitForDeploymentSuccess(ctx context.Context, projectID, deploymentID string) (RailwayDeployment, error) {
	query := `
		query($projectId: String!, $deploymentId: String!) {
			deployment(projectId: $projectId, deploymentId: $deploymentId) {
				id
				status
				url
			}
		}
	`
	for {
		var data struct {
			Deployment RailwayDeployment `json:"deployment"`
		}
		if err := c.doGraphQL(ctx, query, map[string]interface{}{"projectId": projectID, "deploymentId": deploymentID}, &data); err != nil {
			return RailwayDeployment{}, err
		}

		switch data.Deployment.Status {
		case "SUCCESS":
			return data.Deployment, nil
		case "FAILED", "ERROR", "BUILD_ERROR", "DEPLOYMENT_ERROR":
			return data.Deployment, fmt.Errorf("deployment failed with status: %s", data.Deployment.Status)
		}

		if err := sleepWithContext(ctx, railPollInterval); err != nil {
			return RailwayDeployment{}, err
		}
	}
}

type railwayProviderOption struct {
	ProjectName string
	ServiceName string
	Image       string
}

func resolveRailwayOptions(cfg DeployConfig) railwayProviderOption {
	opts := railwayProviderOption{
		ProjectName: strings.TrimSpace(cfg.ProviderOptions[railOptionProjectName]),
		ServiceName: strings.TrimSpace(cfg.ProviderOptions[railOptionServiceName]),
		Image:       strings.TrimSpace(cfg.ProviderOptions[railOptionImage]),
	}
	if opts.ServiceName == "" {
		opts.ServiceName = railDefaultServiceName
	}
	return opts
}

// railwayProvider implements DeployProvider for Railway.
type railwayProvider struct {
	client      RailwayClient
	waitTimeout time.Duration
}

func (p railwayProvider) Name() string {
	return deployProviderRailway
}

func (p railwayProvider) Validate(cfg DeployConfig) error {
	opts := resolveRailwayOptions(cfg)
	if opts.Image == "" {
		return fmt.Errorf("railway deployment requires --image (OCI image URL to deploy)\n  Example: --image ghcr.io/gridlhq/ayb:latest")
	}
	warnMissingDatabaseConfig(cfg)
	return nil
}

// Deploy creates or reuses a Railway project and service, sets environment variables from the configuration, triggers a deployment with the specified image, and waits for the deployment to complete, returning the application URL and dashboard details.
func (p railwayProvider) Deploy(ctx context.Context, cfg DeployConfig) (DeployResult, error) {
	waitCtx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()

	opts := resolveRailwayOptions(cfg)
	mergedEnv, err := mergeDeployEnv(cfg)
	if err != nil {
		return DeployResult{}, fmt.Errorf("merge deploy env: %w", err)
	}

	projectName := opts.ProjectName
	if projectName == "" {
		projectName = deriveAppName(cfg.Domain, "ayb-railway-")
	}

	client := p.client
	if client == nil {
		client = newRailwayHTTPClient(cfg.Token)
	}

	project, err := client.GetOrCreateProject(waitCtx, projectName)
	if err != nil {
		return DeployResult{}, fmt.Errorf("getting/creating project: %w", err)
	}

	service, err := client.GetOrCreateService(waitCtx, project.ID, opts.ServiceName)
	if err != nil {
		return DeployResult{}, fmt.Errorf("getting/creating service: %w", err)
	}

	if err := client.SetConfigVariables(waitCtx, project.ID, mergedEnv); err != nil {
		return DeployResult{}, fmt.Errorf("setting config variables: %w", err)
	}

	deployment, err := client.TriggerDeployment(waitCtx, project.ID, service.ID, opts.Image)
	if err != nil {
		return DeployResult{}, fmt.Errorf("triggering deployment: %w", err)
	}

	finalDeployment, err := client.WaitForDeploymentSuccess(waitCtx, project.ID, deployment.ID)
	if err != nil {
		return DeployResult{}, fmt.Errorf("waiting for deployment success: %w", err)
	}

	appURL := strings.TrimSpace(finalDeployment.URL)
	if appURL == "" {
		appURL = fmt.Sprintf("https://railway.app/project/%s", project.ID)
	}

	return DeployResult{
		Provider:     deployProviderRailway,
		AppURL:       appURL,
		DashboardURL: fmt.Sprintf("https://railway.app/project/%s", project.ID),
		NextSteps: []string{
			"Open Railway dashboard to inspect deployment logs",
			"Configure custom domain in Railway service settings",
			"Set an admin password after first boot: ayb admin reset-password",
		},
		Metadata: map[string]any{
			"project_id":    project.ID,
			"service_id":    service.ID,
			"deployment_id": deployment.ID,
		},
	}, nil
}

func (p railwayProvider) timeout() time.Duration {
	return resolveProviderTimeout(p.waitTimeout, railDefaultWaitTimeout)
}
