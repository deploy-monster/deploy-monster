package marketplace

// LoadBuiltins populates the registry with built-in marketplace templates.
func (r *TemplateRegistry) LoadBuiltins() {
	for _, t := range builtinTemplates {
		r.Add(t)
	}
}

var builtinTemplates = []*Template{
	{
		Slug: "wordpress", Name: "WordPress", Category: "cms", Icon: "📝",
		Description: "The world's most popular content management system",
		Tags:        []string{"blog", "cms", "php"}, Author: "WordPress.org", Version: "6.7",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024, CPUMB: 500},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"DB_PASSWORD": map[string]any{
					"type": "string", "title": "Database Password", "description": "Password for the WordPress database user", "format": "password", "minLength": 8,
				},
				"DB_ROOT_PASSWORD": map[string]any{
					"type": "string", "title": "Database Root Password", "description": "Root password for MariaDB admin", "format": "password", "minLength": 8,
				},
			},
			"required": []string{"DB_PASSWORD", "DB_ROOT_PASSWORD"},
		},
		ComposeYAML: `services:
  wordpress:
    image: wordpress:6.7-apache
    ports: ["80:80"]
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: wordpress
      WORDPRESS_DB_PASSWORD: ${DB_PASSWORD:-changeme}
      WORDPRESS_DB_NAME: wordpress
    volumes: ["wp_data:/var/www/html"]
    depends_on: [db]
  db:
    image: mariadb:11
    environment:
      MARIADB_DATABASE: wordpress
      MARIADB_USER: wordpress
      MARIADB_PASSWORD: ${DB_PASSWORD:-changeme}
      MARIADB_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  wp_data:
  db_data:`,
	},
	{
		Slug: "ghost", Name: "Ghost", Category: "cms", Icon: "👻",
		Description: "Professional publishing platform for blogs and newsletters",
		Tags:        []string{"blog", "newsletter", "nodejs"}, Author: "Ghost Foundation", Version: "5",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512, CPUMB: 500},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"DB_PASSWORD": map[string]any{
					"type": "string", "title": "Database Password", "description": "Password for the Ghost database user", "format": "password", "minLength": 8,
				},
				"DB_ROOT_PASSWORD": map[string]any{
					"type": "string", "title": "Database Root Password", "description": "Root password for MySQL admin", "format": "password", "minLength": 8,
				},
				"SITE_URL": map[string]any{
					"type": "string", "title": "Site URL", "description": "Public URL for your Ghost site", "default": "http://localhost:2368",
				},
			},
			"required": []string{"DB_PASSWORD", "DB_ROOT_PASSWORD"},
		},
		ComposeYAML: `services:
  ghost:
    image: ghost:5-alpine
    ports: ["2368:2368"]
    environment:
      url: ${SITE_URL:-http://localhost:2368}
      database__client: mysql
      database__connection__host: db
      database__connection__user: ghost
      database__connection__password: ${DB_PASSWORD:-changeme}
      database__connection__database: ghost
    volumes: ["ghost_data:/var/lib/ghost/content"]
    depends_on: [db]
  db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: ghost
      MYSQL_USER: ghost
      MYSQL_PASSWORD: ${DB_PASSWORD:-changeme}
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  ghost_data:
  db_data:`,
	},
	{
		Slug: "n8n", Name: "n8n", Category: "automation", Icon: "⚡",
		Description: "Workflow automation tool — open-source alternative to Zapier",
		Tags:        []string{"automation", "workflow", "integration"}, Author: "n8n GmbH", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 256, CPUMB: 500},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"N8N_USER": map[string]any{
					"type": "string", "title": "Admin Username", "description": "Username for n8n admin login", "default": "admin",
				},
				"N8N_PASSWORD": map[string]any{
					"type": "string", "title": "Admin Password", "description": "Password for n8n admin login", "format": "password", "minLength": 8,
				},
			},
			"required": []string{"N8N_PASSWORD"},
		},
		ComposeYAML: `services:
  n8n:
    image: n8nio/n8n:latest
    ports: ["5678:5678"]
    environment:
      N8N_BASIC_AUTH_ACTIVE: "true"
      N8N_BASIC_AUTH_USER: ${N8N_USER:-admin}
      N8N_BASIC_AUTH_PASSWORD: ${N8N_PASSWORD:-changeme}
    volumes: ["n8n_data:/home/node/.n8n"]
volumes:
  n8n_data:`,
	},
	{
		Slug: "uptime-kuma", Name: "Uptime Kuma", Category: "monitoring", Icon: "📊",
		Description: "Self-hosted monitoring tool like Uptime Robot",
		Tags:        []string{"monitoring", "uptime", "status"}, Author: "louislam", Version: "1",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256, CPUMB: 250},
		ComposeYAML: `services:
  uptime-kuma:
    image: louislam/uptime-kuma:1
    ports: ["3001:3001"]
    volumes: ["kuma_data:/app/data"]
volumes:
  kuma_data:`,
	},
	{
		Slug: "gitea", Name: "Gitea", Category: "devtools", Icon: "🔀",
		Description: "Lightweight self-hosted Git service",
		Tags:        []string{"git", "scm", "devops"}, Author: "Gitea", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512, CPUMB: 250},
		ComposeYAML: `services:
  gitea:
    image: gitea/gitea:latest
    ports: ["3000:3000", "2222:22"]
    environment:
      GITEA__database__DB_TYPE: sqlite3
    volumes: ["gitea_data:/data"]
volumes:
  gitea_data:`,
	},
	{
		Slug: "minio", Name: "MinIO", Category: "storage", Icon: "🪣",
		Description: "High-performance S3-compatible object storage",
		Tags:        []string{"s3", "storage", "object"}, Author: "MinIO Inc", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024, CPUMB: 500},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"MINIO_USER": map[string]any{
					"type": "string", "title": "Admin Username", "description": "MinIO root username", "default": "minioadmin",
				},
				"MINIO_PASSWORD": map[string]any{
					"type": "string", "title": "Admin Password", "description": "MinIO root password", "format": "password", "minLength": 8,
				},
			},
			"required": []string{"MINIO_PASSWORD"},
		},
		ComposeYAML: `services:
  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    ports: ["9000:9000", "9001:9001"]
    environment:
      MINIO_ROOT_USER: ${MINIO_USER:-minioadmin}
      MINIO_ROOT_PASSWORD: ${MINIO_PASSWORD:-minioadmin}
    volumes: ["minio_data:/data"]
volumes:
  minio_data:`,
	},
	{
		Slug: "plausible", Name: "Plausible Analytics", Category: "analytics", Icon: "📈",
		Description: "Privacy-friendly Google Analytics alternative",
		Tags:        []string{"analytics", "privacy", "web"}, Author: "Plausible", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048, CPUMB: 500},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"SECRET_KEY": map[string]any{
					"type": "string", "title": "Secret Key", "description": "Encryption key for sessions and cookies", "format": "password", "minLength": 16,
				},
				"BASE_URL": map[string]any{
					"type": "string", "title": "Base URL", "description": "Public URL for your Plausible instance", "default": "http://localhost:8000",
				},
			},
			"required": []string{"SECRET_KEY"},
		},
		ComposeYAML: `services:
  plausible:
    image: ghcr.io/plausible/community-edition:latest
    ports: ["8000:8000"]
    environment:
      BASE_URL: ${BASE_URL:-http://localhost:8000}
      SECRET_KEY_BASE: ${SECRET_KEY:-please-change-me}
    volumes: ["plausible_data:/var/lib/plausible"]
volumes:
  plausible_data:`,
	},
	{
		Slug: "vaultwarden", Name: "Vaultwarden", Category: "security", Icon: "🔐",
		Description: "Lightweight Bitwarden-compatible password manager",
		Tags:        []string{"password", "security", "bitwarden"}, Author: "dani-garcia", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 128, DiskMB: 256, CPUMB: 250},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ADMIN_TOKEN": map[string]any{
					"type": "string", "title": "Admin Token", "description": "Secret token for admin access", "format": "password", "minLength": 16,
				},
			},
			"required": []string{"ADMIN_TOKEN"},
		},
		ComposeYAML: `services:
  vaultwarden:
    image: vaultwarden/server:latest
    ports: ["80:80"]
    environment:
      ADMIN_TOKEN: ${ADMIN_TOKEN:-changeme}
    volumes: ["vw_data:/data"]
volumes:
  vw_data:`,
	},
	{
		Slug: "nextcloud", Name: "Nextcloud", Category: "storage", Icon: "☁️",
		Description: "Self-hosted file sync, sharing, and collaboration platform",
		Tags:        []string{"cloud", "files", "collaboration"}, Author: "Nextcloud", Version: "29",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 5120, CPUMB: 1000},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"DB_PASSWORD": map[string]any{
					"type": "string", "title": "Database Password", "description": "Password for the Nextcloud database user", "format": "password", "minLength": 8,
				},
				"DB_ROOT_PASSWORD": map[string]any{
					"type": "string", "title": "Database Root Password", "description": "Root password for MariaDB admin", "format": "password", "minLength": 8,
				},
			},
			"required": []string{"DB_PASSWORD", "DB_ROOT_PASSWORD"},
		},
		ComposeYAML: `services:
  nextcloud:
    image: nextcloud:29-apache
    ports: ["80:80"]
    environment:
      MYSQL_HOST: db
      MYSQL_DATABASE: nextcloud
      MYSQL_USER: nextcloud
      MYSQL_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["nc_data:/var/www/html"]
    depends_on: [db]
  db:
    image: mariadb:11
    environment:
      MARIADB_DATABASE: nextcloud
      MARIADB_USER: nextcloud
      MARIADB_PASSWORD: ${DB_PASSWORD:-changeme}
      MARIADB_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  nc_data:
  db_data:`,
	},
	{
		Slug: "metabase", Name: "Metabase", Category: "analytics", Icon: "📊",
		Description: "Business intelligence and analytics dashboard",
		Tags:        []string{"bi", "analytics", "dashboard"}, Author: "Metabase", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 512, CPUMB: 500},
		ComposeYAML: `services:
  metabase:
    image: metabase/metabase:latest
    ports: ["3000:3000"]
    volumes: ["metabase_data:/metabase-data"]
    environment:
      MB_DB_FILE: /metabase-data/metabase.db
volumes:
  metabase_data:`,
	},
	{
		Slug: "ollama", Name: "Ollama", Category: "ai", Icon: "🤖",
		Description: "Run large language models locally",
		Tags:        []string{"ai", "llm", "ml"}, Author: "Ollama", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 4096, DiskMB: 10240, CPUMB: 2000},
		ComposeYAML: `services:
  ollama:
    image: ollama/ollama:latest
    ports: ["11434:11434"]
    volumes: ["ollama_data:/root/.ollama"]
volumes:
  ollama_data:`,
	},
	{
		Slug: "code-server", Name: "Code Server", Category: "devtools", Icon: "💻",
		Description: "VS Code in the browser",
		Tags:        []string{"ide", "vscode", "development"}, Author: "Coder", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048, CPUMB: 1000},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"PASSWORD": map[string]any{
					"type": "string", "title": "IDE Password", "description": "Password to access VS Code in browser", "format": "password", "minLength": 8,
				},
			},
			"required": []string{"PASSWORD"},
		},
		ComposeYAML: `services:
  code-server:
    image: codercom/code-server:latest
    ports: ["8080:8080"]
    environment:
      PASSWORD: ${PASSWORD:-changeme}
    volumes: ["code_data:/home/coder"]
volumes:
  code_data:`,
	},
}
