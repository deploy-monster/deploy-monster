package mcp

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema defines the JSON Schema for tool input.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property defines a single input property.
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

// BuiltinTools returns all MCP tools DeployMonster exposes.
func BuiltinTools() []Tool {
	return []Tool{
		{
			Name: "deploy_app", Description: "Deploy a new application from a Docker image or Git repository",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"name":        {Type: "string", Description: "Application name"},
					"source_type": {Type: "string", Description: "Source type", Enum: []string{"image", "git", "compose"}},
					"source_url":  {Type: "string", Description: "Docker image or Git URL"},
				},
				Required: []string{"name", "source_type", "source_url"},
			},
		},
		{
			Name: "list_apps", Description: "List all deployed applications",
			InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
		},
		{
			Name: "get_app_status", Description: "Get the current status of an application",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"app_id": {Type: "string", Description: "Application ID"},
				},
				Required: []string{"app_id"},
			},
		},
		{
			Name: "scale_app", Description: "Scale an application's replica count",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"app_id":   {Type: "string", Description: "Application ID"},
					"replicas": {Type: "integer", Description: "Number of replicas"},
				},
				Required: []string{"app_id", "replicas"},
			},
		},
		{
			Name: "view_logs", Description: "View recent logs for an application",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"app_id": {Type: "string", Description: "Application ID"},
					"lines":  {Type: "integer", Description: "Number of log lines to return"},
				},
				Required: []string{"app_id"},
			},
		},
		{
			Name: "create_database", Description: "Create a new managed database",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"name":    {Type: "string", Description: "Database name"},
					"engine":  {Type: "string", Description: "Database engine", Enum: []string{"postgres", "mysql", "redis", "mongodb"}},
					"version": {Type: "string", Description: "Engine version"},
				},
				Required: []string{"name", "engine"},
			},
		},
		{
			Name: "add_domain", Description: "Add a custom domain to an application",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"app_id": {Type: "string", Description: "Application ID"},
					"fqdn":   {Type: "string", Description: "Fully qualified domain name"},
				},
				Required: []string{"app_id", "fqdn"},
			},
		},
		{
			Name: "marketplace_deploy", Description: "Deploy an application from the marketplace",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"template": {Type: "string", Description: "Template slug (e.g. wordpress, ghost, n8n)"},
				},
				Required: []string{"template"},
			},
		},
		{
			Name: "provision_server", Description: "Provision a new VPS server",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"provider": {Type: "string", Description: "Cloud provider", Enum: []string{"hetzner", "digitalocean", "vultr"}},
					"name":     {Type: "string", Description: "Server name"},
					"region":   {Type: "string", Description: "Region/location"},
					"size":     {Type: "string", Description: "Server size/plan"},
				},
				Required: []string{"provider", "name", "region", "size"},
			},
		},
	}
}
