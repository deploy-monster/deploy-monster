package marketplace

// LoadBuiltins populates the registry with built-in marketplace templates.
// Curated entries (with full ConfigSchema, icons, resource minimums) appear
// first in builtinTemplates; bulk-imported entries follow. When two entries
// share a slug, the first (curated) entry wins — later duplicates are skipped.
func (r *TemplateRegistry) LoadBuiltins() {
	for _, t := range builtinTemplates {
		if r.Get(t.Slug) != nil {
			continue // curated entry already registered; skip bulk duplicate
		}
		r.Add(t)
	}
}

// builtinTemplates is the single source of truth for all built-in marketplace
// templates. Curated entries (with full ConfigSchema, icons, resource minimums)
// appear first; bulk-imported entries follow. When two entries share a slug,
// LoadBuiltins registers only the first (curated) one via registry.Add's
// map semantics.
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
	// === Databases ===
	{
		Slug: "postgresql", Name: "PostgreSQL", Category: "database",
		Description: "The world's most advanced open source relational database",
		Tags:        []string{"database", "sql", "relational"}, Author: "PostgreSQL", Version: "17",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  postgres:
    image: postgres:17-alpine
    ports: ["5432:5432"]
    environment:
      POSTGRES_USER: ${DB_USER:-postgres}
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
      POSTGRES_DB: ${DB_NAME:-app}
    volumes: ["pg_data:/var/lib/postgresql/data"]
volumes:
  pg_data:`,
	},
	{
		Slug: "mysql", Name: "MySQL", Category: "database",
		Description: "The world's most popular open source database",
		Tags:        []string{"database", "sql", "relational"}, Author: "Oracle", Version: "8.4",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  mysql:
    image: mysql:8.4
    ports: ["3306:3306"]
    environment:
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
      MYSQL_DATABASE: ${DB_NAME:-app}
      MYSQL_USER: ${DB_USER:-app}
      MYSQL_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["mysql_data:/var/lib/mysql"]
volumes:
  mysql_data:`,
	},
	{
		Slug: "redis", Name: "Redis", Category: "database",
		Description: "In-memory data structure store, cache, and message broker",
		Tags:        []string{"cache", "database", "nosql"}, Author: "Redis Ltd", Version: "7",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 128, DiskMB: 128},
		ComposeYAML: `services:
  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
    command: redis-server --appendonly yes
    volumes: ["redis_data:/data"]
volumes:
  redis_data:`,
	},
	{
		Slug: "mongodb", Name: "MongoDB", Category: "database",
		Description: "Document-oriented NoSQL database",
		Tags:        []string{"database", "nosql", "document"}, Author: "MongoDB Inc", Version: "8",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024},
		ComposeYAML: `services:
  mongo:
    image: mongo:8
    ports: ["27017:27017"]
    environment:
      MONGO_INITDB_ROOT_USERNAME: ${DB_USER:-admin}
      MONGO_INITDB_ROOT_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["mongo_data:/data/db"]
volumes:
  mongo_data:`,
	},
	{
		Slug: "elasticsearch", Name: "Elasticsearch", Category: "search",
		Description: "Distributed search and analytics engine",
		Tags:        []string{"search", "analytics", "logging"}, Author: "Elastic", Version: "8",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 2048, DiskMB: 2048},
		ComposeYAML: `services:
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.12.0
    ports: ["9200:9200"]
    environment:
      discovery.type: single-node
      xpack.security.enabled: "false"
      ES_JAVA_OPTS: -Xms512m -Xmx512m
    volumes: ["es_data:/usr/share/elasticsearch/data"]
volumes:
  es_data:`,
	},
	// === Development Tools ===
	{
		Slug: "gitlab", Name: "GitLab CE", Category: "devtools",
		Description: "Complete DevOps platform in a single application",
		Tags:        []string{"git", "ci/cd", "devops"}, Author: "GitLab", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 4096, DiskMB: 10240},
		ComposeYAML: `services:
  gitlab:
    image: gitlab/gitlab-ce:latest
    ports: ["80:80", "443:443", "2222:22"]
    environment:
      GITLAB_OMNIBUS_CONFIG: |
        external_url 'http://localhost'
        gitlab_rails['initial_root_password'] = '${ROOT_PASSWORD:-changeme}'
    volumes: ["gitlab_config:/etc/gitlab", "gitlab_logs:/var/log/gitlab", "gitlab_data:/var/opt/gitlab"]
volumes:
  gitlab_config:
  gitlab_logs:
  gitlab_data:`,
	},
	{
		Slug: "drone", Name: "Drone CI", Category: "devtools",
		Description: "Container-native continuous integration platform",
		Tags:        []string{"ci", "cd", "containers"}, Author: "Harness", Version: "2",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
		ComposeYAML: `services:
  drone:
    image: drone/drone:2
    ports: ["80:80"]
    environment:
      DRONE_SERVER_HOST: ${HOST:-localhost}
      DRONE_SERVER_PROTO: http
      DRONE_RPC_SECRET: ${SECRET:-change-this}
    volumes: ["drone_data:/data"]
volumes:
  drone_data:`,
	},
	{
		Slug: "woodpecker", Name: "Woodpecker CI", Category: "devtools",
		Description: "Simple CI engine with great extensibility",
		Tags:        []string{"ci", "cd", "automation"}, Author: "Woodpecker", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
		ComposeYAML: `services:
  woodpecker:
    image: woodpeckerci/woodpecker-server:latest
    ports: ["8000:8000"]
    environment:
      WOODPECKER_OPEN: "true"
      WOODPECKER_HOST: ${HOST:-http://localhost:8000}
      WOODPECKER_AGENT_SECRET: ${SECRET:-change-this}
    volumes: ["woodpecker_data:/var/lib/woodpecker"]
volumes:
  woodpecker_data:`,
	},
	// === Media & Entertainment ===
	{
		Slug: "jellyfin", Name: "Jellyfin", Category: "media",
		Description: "Free software media system — Plex alternative",
		Tags:        []string{"media", "streaming", "movies"}, Author: "Jellyfin", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
		ComposeYAML: `services:
  jellyfin:
    image: jellyfin/jellyfin:latest
    ports: ["8096:8096"]
    volumes:
      - jellyfin_config:/config
      - jellyfin_cache:/cache
      - media:/media
volumes:
  jellyfin_config:
  jellyfin_cache:
  media:`,
	},
	{
		Slug: "navidrome", Name: "Navidrome", Category: "media",
		Description: "Modern music server and streamer — Subsonic compatible",
		Tags:        []string{"music", "streaming", "audio"}, Author: "Navidrome", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  navidrome:
    image: deluan/navidrome:latest
    ports: ["4533:4533"]
    environment:
      ND_SCANSCHEDULE: "1h"
    volumes:
      - navidrome_data:/data
      - music:/music
volumes:
  navidrome_data:
  music:`,
	},
	{
		Slug: "audiobookshelf", Name: "Audiobookshelf", Category: "media",
		Description: "Self-hosted audiobook and podcast server",
		Tags:        []string{"audiobooks", "podcasts", "media"}, Author: "advplyr", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024},
		ComposeYAML: `services:
  audiobookshelf:
    image: ghcr.io/advplyr/audiobookshelf:latest
    ports: ["13378:80"]
    volumes:
      - audiobooks:/audiobooks
      - podcasts:/podcasts
      - config:/config
      - metadata:/metadata
volumes:
  audiobooks:
  podcasts:
  config:
  metadata:`,
	},
	// === Communication ===
	{
		Slug: "mattermost", Name: "Mattermost", Category: "communication",
		Description: "Open-source Slack alternative for developers",
		Tags:        []string{"chat", "team", "messaging"}, Author: "Mattermost", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
		ComposeYAML: `services:
  mattermost:
    image: mattermost/mattermost-team-edition:latest
    ports: ["8065:8065"]
    environment:
      MM_SQLSETTINGS_DRIVERNAME: postgres
      MM_SQLSETTINGS_DATASOURCE: postgres://mmuser:${DB_PASSWORD:-changeme}@db:5432/mattermost?sslmode=disable
    volumes: ["mm_data:/mattermost/data"]
    depends_on: [db]
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: mattermost
      POSTGRES_USER: mmuser
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  mm_data:
  db_data:`,
	},
	{
		Slug: "rocketchat", Name: "Rocket.Chat", Category: "communication",
		Description: "Open-source team chat platform",
		Tags:        []string{"chat", "team", "messaging"}, Author: "Rocket.Chat", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
		ComposeYAML: `services:
  rocketchat:
    image: rocket.chat:latest
    ports: ["3000:3000"]
    environment:
      ROOT_URL: ${URL:-http://localhost:3000}
      MONGO_URL: mongodb://db:27017/rocketchat
      MONGO_OPLOG_URL: mongodb://db:27017/local
    depends_on: [db]
  db:
    image: mongo:8
    command: mongod --replSet rs0 --oplogSize 128
    volumes: ["db_data:/data/db"]
volumes:
  db_data:`,
	},
	{
		Slug: "matrix-synapse", Name: "Matrix Synapse", Category: "communication",
		Description: "Decentralized, secure messaging homeserver",
		Tags:        []string{"matrix", "chat", "federation"}, Author: "Matrix.org", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024},
		ComposeYAML: `services:
  synapse:
    image: matrixorg/synapse:latest
    ports: ["8008:8008"]
    environment:
      SYNAPSE_SERVER_NAME: ${SERVER_NAME:-localhost}
      SYNAPSE_REPORT_STATS: "no"
    volumes: ["synapse_data:/data"]
volumes:
  synapse_data:`,
	},
	// === Productivity ===
	{
		Slug: "onlyoffice", Name: "ONLYOFFICE Docs", Category: "productivity",
		Description: "Office suite for document editing — Google Docs alternative",
		Tags:        []string{"office", "documents", "collaboration"}, Author: "ONLYOFFICE", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 2048, DiskMB: 2048},
		ComposeYAML: `services:
  onlyoffice:
    image: onlyoffice/documentserver:latest
    ports: ["80:80"]
    environment:
      JWT_ENABLED: "false"
    volumes:
      - onlyoffice_data:/var/www/onlyoffice/Data
      - onlyoffice_logs:/var/log/onlyoffice
volumes:
  onlyoffice_data:
  onlyoffice_logs:`,
	},
	{
		Slug: "collabora", Name: "Collabora Online", Category: "productivity",
		Description: "LibreOffice-based online office suite",
		Tags:        []string{"office", "documents", "collaboration"}, Author: "Collabora", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 512},
		ComposeYAML: `services:
  collabora:
    image: collabora/code:latest
    ports: ["9980:9980"]
    environment:
      domain: ${DOMAIN:-localhost}
      dictionaries: en
    volumes: ["collabora_data:/opt/collabora"]
volumes:
  collabora_data:`,
	},
	{
		Slug: "outline", Name: "Outline", Category: "productivity",
		Description: "Modern team knowledge base — Notion alternative",
		Tags:        []string{"wiki", "knowledge", "documentation"}, Author: "Outline", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  outline:
    image: outlinewiki/outline:latest
    ports: ["3000:3000"]
    environment:
      DATABASE_URL: postgres://outline:${DB_PASSWORD:-changeme}@db:5432/outline
      REDIS_URL: redis://redis:6379
      SECRET_KEY: ${SECRET:-change-this-key}
      UTILS_SECRET: ${UTILS_SECRET:-change-this-too}
    depends_on: [db, redis]
  redis:
    image: redis:7-alpine
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: outline
      POSTGRES_USER: outline
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	// === Monitoring & Observability ===
	{
		Slug: "prometheus", Name: "Prometheus", Category: "monitoring",
		Description: "Systems monitoring and alerting toolkit",
		Tags:        []string{"monitoring", "metrics", "alerting"}, Author: "Prometheus", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  prometheus:
    image: prom/prometheus:latest
    ports: ["9090:9090"]
    command: --config.file=/etc/prometheus/prometheus.yml
    volumes:
      - prometheus_data:/prometheus
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
volumes:
  prometheus_data:`,
	},
	{
		Slug: "loki", Name: "Loki", Category: "monitoring",
		Description: "Log aggregation system designed for efficiency",
		Tags:        []string{"logging", "observability", "grafana"}, Author: "Grafana Labs", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  loki:
    image: grafana/loki:latest
    ports: ["3100:3100"]
    command: -config.file=/etc/loki/local-config.yaml
    volumes: ["loki_data:/loki"]
volumes:
  loki_data:`,
	},
	{
		Slug: "tempo", Name: "Tempo", Category: "monitoring",
		Description: "Distributed tracing backend",
		Tags:        []string{"tracing", "observability", "grafana"}, Author: "Grafana Labs", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  tempo:
    image: grafana/tempo:latest
    ports: ["3200:3200"]
    command: -config.file=/etc/tempo.yaml
    volumes: ["tempo_data:/var/tempo"]
volumes:
  tempo_data:`,
	},
	// === Security ===
	{
		Slug: "traefik", Name: "Traefik", Category: "networking",
		Description: "Modern HTTP reverse proxy and load balancer",
		Tags:        []string{"proxy", "load-balancer", "ssl"}, Author: "Traefik Labs", Version: "3",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 128, DiskMB: 128},
		ComposeYAML: `services:
  traefik:
    image: traefik:v3
    ports: ["80:80", "443:443", "8080:8080"]
    command:
      - --api.insecure=true
      - --providers.docker=true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - traefik_data:/data
volumes:
  traefik_data:`,
	},
	{
		Slug: "caddy", Name: "Caddy", Category: "networking",
		Description: "Powerful web server with automatic HTTPS",
		Tags:        []string{"web-server", "ssl", "proxy"}, Author: "Caddy", Version: "2",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 64, DiskMB: 64},
		ComposeYAML: `services:
  caddy:
    image: caddy:2
    ports: ["80:80", "443:443", "443:443/udp"]
    volumes:
      - caddy_data:/data
      - caddy_config:/config
      - ./Caddyfile:/etc/caddy/Caddyfile
volumes:
  caddy_data:
  caddy_config:`,
	},
	{
		Slug: "authentik", Name: "Authentik", Category: "security",
		Description: "Open-source identity provider — Okta/Auth0 alternative",
		Tags:        []string{"sso", "oauth", "authentication"}, Author: "goauthentik", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
		ComposeYAML: `services:
  authentik-server:
    image: ghcr.io/goauthentik/server:latest
    ports: ["9000:9000", "9443:9443"]
    environment:
      AUTHENTIK_SECRET_KEY: ${SECRET_KEY:-change-this}
      AUTHENTIK_REDIS__HOST: redis
      AUTHENTIK_POSTGRESQL__HOST: db
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: ${DB_PASSWORD:-changeme}
      AUTHENTIK_POSTGRESQL__NAME: authentik
    depends_on: [db, redis]
  worker:
    image: ghcr.io/goauthentik/server:latest
    command: worker
    environment:
      AUTHENTIK_SECRET_KEY: ${SECRET_KEY:-change-this}
      AUTHENTIK_REDIS__HOST: redis
      AUTHENTIK_POSTGRESQL__HOST: db
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: ${DB_PASSWORD:-changeme}
      AUTHENTIK_POSTGRESQL__NAME: authentik
  redis:
    image: redis:7-alpine
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: authentik
      POSTGRES_USER: authentik
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	// === AI & ML ===
	{
		Slug: "openai-whisper", Name: "Whisper ASR", Category: "ai",
		Description: "OpenAI's speech recognition model for transcription",
		Tags:        []string{"ai", "speech", "transcription"}, Author: "OpenAI", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 2048, DiskMB: 4096},
		ComposeYAML: `services:
  whisper:
    image: onerahmet/openai-whisper-asr-webservice:latest
    ports: ["9000:9000"]
    environment:
      ASR_MODEL: base
    volumes: ["whisper_data:/root/.cache"]
volumes:
  whisper_data:`,
	},
	{
		Slug: "stable-diffusion", Name: "Stable Diffusion WebUI", Category: "ai",
		Description: "Image generation with Stable Diffusion models",
		Tags:        []string{"ai", "image", "generation"}, Author: "AUTOMATIC1111", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 8192, DiskMB: 20480},
		ComposeYAML: `services:
  sd-webui:
    image: siutin/stable-diffusion-webui-docker:latest
    ports: ["7860:7860"]
    environment:
      CLI_ARGS: --xformers --listen
    volumes:
      - sd_models:/models
      - sd_outputs:/outputs
volumes:
  sd_models:
  sd_outputs:`,
	},
	{
		Slug: "langflow", Name: "LangFlow", Category: "ai",
		Description: "Visual framework for building multi-agent and RAG applications",
		Tags:        []string{"ai", "langchain", "rag"}, Author: "DataStax", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 2048, DiskMB: 2048},
		ComposeYAML: `services:
  langflow:
    image: langflowai/langflow:latest
    ports: ["7860:7860"]
    environment:
      LANGFLOW_DATABASE_URL: sqlite:////app/data/langflow.db
    volumes: ["langflow_data:/app/data"]
volumes:
  langflow_data:`,
	},
	// === Miscellaneous ===
	{
		Slug: "dokuwiki", Name: "DokuWiki", Category: "collaboration",
		Description: "Simple to use and highly versatile wiki",
		Tags:        []string{"wiki", "documentation", "knowledge"}, Author: "DokuWiki", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 128, DiskMB: 256},
		ComposeYAML: `services:
  dokuwiki:
    image: linuxserver/dokuwiki:latest
    ports: ["80:80"]
    environment:
      PUID: 1000
      PGID: 1000
    volumes: ["wiki_data:/config"]
volumes:
  wiki_data:`,
	},
	{
		Slug: "bookstack", Name: "BookStack", Category: "collaboration",
		Description: "Simple, self-hosted wiki platform",
		Tags:        []string{"wiki", "documentation", "knowledge"}, Author: "BookStack", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  bookstack:
    image: lscr.io/linuxserver/bookstack:latest
    ports: ["80:80"]
    environment:
      DB_HOST: db
      DB_USER: bookstack
      DB_PASS: ${DB_PASSWORD:-changeme}
      DB_DATABASE: bookstack
    depends_on: [db]
  db:
    image: mariadb:11
    environment:
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
      MYSQL_DATABASE: bookstack
      MYSQL_USER: bookstack
      MYSQL_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  db_data:`,
	},
	{
		Slug: "appwrite", Name: "Appwrite", Category: "devtools",
		Description: "Backend-as-a-Service for web and mobile developers",
		Tags:        []string{"backend", "baas", "firebase-alternative"}, Author: "Appwrite", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
		ComposeYAML: `services:
  appwrite:
    image: appwrite/appwrite:latest
    ports: ["80:80", "443:443"]
    volumes:
      - appwrite_storage:/storage
      - appwrite_certs:/certs
volumes:
  appwrite_storage:
  appwrite_certs:`,
	},
	{
		Slug: "pocketbase", Name: "PocketBase", Category: "devtools",
		Description: "Open Source backend in 1 file written in Go",
		Tags:        []string{"backend", "baas", "sqlite"}, Author: "PocketBase", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 64, DiskMB: 128},
		ComposeYAML: `services:
  pocketbase:
    image: ghcr.io/muchobien/pocketbase:latest
    ports: ["8090:8090"]
    volumes: ["pb_data:/pb_data"]
volumes:
  pb_data:`,
	},
	{
		Slug: "supabase", Name: "Supabase", Category: "devtools",
		Description: "Open Source Firebase Alternative with PostgreSQL",
		Tags:        []string{"backend", "baas", "postgresql"}, Author: "Supabase", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 2048, DiskMB: 4096},
		ComposeYAML: `services:
  studio:
    image: supabase/studio:latest
    ports: ["3000:3000"]
    environment:
      SUPABASE_URL: http://kong:8000
  kong:
    image: kong:2.8.1
    ports: ["8000:8000", "8443:8443"]
  db:
    image: supabase/postgres:15.1.0.117
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	{
		Slug: "strapi", Name: "Strapi", Category: "cms", Icon: "📦",
		Description: "Open-source headless CMS built with Node.js",
		Tags:        []string{"cms", "headless", "api"}, Author: "Strapi", Version: "5",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 1024, CPUMB: 1000},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"DB_PASSWORD": map[string]any{
					"type": "string", "title": "Database Password", "description": "Password for the Strapi PostgreSQL user", "format": "password", "minLength": 8,
				},
			},
			"required": []string{"DB_PASSWORD"},
		},
		ComposeYAML: `services:
  strapi:
    image: strapi/strapi:latest
    ports: ["1337:1337"]
    environment:
      DATABASE_CLIENT: postgres
      DATABASE_HOST: db
      DATABASE_NAME: strapi
      DATABASE_USERNAME: strapi
      DATABASE_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["strapi_data:/srv/app"]
    depends_on: [db]
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: strapi
      POSTGRES_USER: strapi
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  strapi_data:
  db_data:`,
	},
	{
		Slug: "umami", Name: "Umami", Category: "analytics", Icon: "📊",
		Description: "Simple, fast, privacy-focused web analytics",
		Tags:        []string{"analytics", "privacy"}, Author: "Umami", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512, CPUMB: 500},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"DB_PASSWORD": map[string]any{
					"type": "string", "title": "Database Password", "description": "Password for the Umami PostgreSQL user", "format": "password", "minLength": 8,
				},
			},
			"required": []string{"DB_PASSWORD"},
		},
		ComposeYAML: `services:
  umami:
    image: ghcr.io/umami-software/umami:postgresql-latest
    ports: ["3000:3000"]
    environment:
      DATABASE_URL: postgres://umami:${DB_PASSWORD:-changeme}@db:5432/umami
      DATABASE_TYPE: postgresql
    depends_on: [db]
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: umami
      POSTGRES_USER: umami
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	{
		Slug: "open-webui", Name: "Open WebUI", Category: "ai", Icon: "💬",
		Description: "Self-hosted ChatGPT-like interface for Ollama and OpenAI",
		Tags:        []string{"ai", "chat", "llm"}, Author: "Open WebUI", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048, CPUMB: 500},
		ComposeYAML: `services:
  open-webui:
    image: ghcr.io/open-webui/open-webui:main
    ports: ["8080:8080"]
    environment:
      OLLAMA_BASE_URL: ${OLLAMA_URL:-http://ollama:11434}
    volumes: ["webui_data:/app/backend/data"]
volumes:
  webui_data:`,
	},
	{
		Slug: "meilisearch", Name: "Meilisearch", Category: "search", Icon: "🔍",
		Description: "Lightning-fast search engine — open-source alternative to Algolia",
		Tags:        []string{"search", "api"}, Author: "Meilisearch", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512, CPUMB: 500},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"MASTER_KEY": map[string]any{
					"type": "string", "title": "Master Key", "description": "API master key for Meilisearch", "format": "password", "minLength": 16,
				},
			},
			"required": []string{"MASTER_KEY"},
		},
		ComposeYAML: `services:
  meilisearch:
    image: getmeili/meilisearch:latest
    ports: ["7700:7700"]
    environment:
      MEILI_MASTER_KEY: ${MASTER_KEY:-masterKey123}
    volumes: ["meili_data:/meili_data"]
volumes:
  meili_data:`,
	},
	{
		Slug: "nocodb", Name: "NocoDB", Category: "database", Icon: "🗂️",
		Description: "Open-source Airtable alternative — turn any database into a spreadsheet",
		Tags:        []string{"database", "spreadsheet", "no-code"}, Author: "NocoDB", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512, CPUMB: 250},
		ComposeYAML: `services:
  nocodb:
    image: nocodb/nocodb:latest
    ports: ["8080:8080"]
    volumes: ["nocodb_data:/usr/app/data"]
volumes:
  nocodb_data:`,
	},
	{
		Slug: "portainer", Name: "Portainer CE", Category: "devtools", Icon: "🐳",
		Description: "Container management UI for Docker and Kubernetes",
		Tags:        []string{"docker", "containers", "management"}, Author: "Portainer", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256, CPUMB: 250},
		ComposeYAML: `services:
  portainer:
    image: portainer/portainer-ce:latest
    ports: ["9443:9443"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - portainer_data:/data
volumes:
  portainer_data:`,
	},
	{
		Slug: "grafana", Name: "Grafana", Category: "monitoring", Icon: "📊",
		Description: "Open-source observability and monitoring dashboards",
		Tags:        []string{"monitoring", "dashboards", "visualization"}, Author: "Grafana Labs", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512, CPUMB: 500},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ADMIN_PASSWORD": map[string]any{
					"type": "string", "title": "Admin Password", "description": "Password for the Grafana admin user", "format": "password", "minLength": 8,
				},
			},
			"required": []string{"ADMIN_PASSWORD"},
		},
		ComposeYAML: `services:
  grafana:
    image: grafana/grafana-oss:latest
    ports: ["3000:3000"]
    environment:
      GF_SECURITY_ADMIN_PASSWORD: ${ADMIN_PASSWORD:-admin}
    volumes: ["grafana_data:/var/lib/grafana"]
volumes:
  grafana_data:`,
	},
	{
		Slug: "directus", Name: "Directus", Category: "cms", Icon: "🔷",
		Description: "Open-source headless CMS with real-time API and beautiful admin",
		Tags:        []string{"cms", "headless", "api", "admin"}, Author: "Directus", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024, CPUMB: 500},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"DB_PASSWORD": map[string]any{
					"type": "string", "title": "Database Password", "description": "Password for the Directus PostgreSQL user", "format": "password", "minLength": 8,
				},
				"SECRET": map[string]any{
					"type": "string", "title": "Secret Key", "description": "Encryption key for Directus", "format": "password", "minLength": 16,
				},
				"ADMIN_EMAIL": map[string]any{
					"type": "string", "title": "Admin Email", "description": "Email for the Directus admin account",
				},
				"ADMIN_PASSWORD": map[string]any{
					"type": "string", "title": "Admin Password", "description": "Password for the Directus admin account", "format": "password", "minLength": 8,
				},
			},
			"required": []string{"DB_PASSWORD", "SECRET", "ADMIN_PASSWORD"},
		},
		ComposeYAML: `services:
  directus:
    image: directus/directus:latest
    ports: ["8055:8055"]
    environment:
      SECRET: ${SECRET:-change-this-secret}
      ADMIN_EMAIL: ${ADMIN_EMAIL:-admin@example.com}
      ADMIN_PASSWORD: ${ADMIN_PASSWORD:-changeme}
      DB_CLIENT: pg
      DB_HOST: db
      DB_PORT: 5432
      DB_DATABASE: directus
      DB_USER: directus
      DB_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["directus_data:/directus/uploads"]
    depends_on: [db]
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: directus
      POSTGRES_USER: directus
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  directus_data:
  db_data:`,
	},
	{
		Slug: "immich", Name: "Immich", Category: "storage",
		Description: "Self-hosted photo and video backup — Google Photos alternative",
		Tags:        []string{"photos", "backup", "media"}, Author: "Immich", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 2048, DiskMB: 10240},
		ComposeYAML: `services:
  immich:
    image: ghcr.io/immich-app/immich-server:release
    ports: ["2283:2283"]
    environment:
      DB_HOSTNAME: db
      DB_USERNAME: immich
      DB_PASSWORD: ${DB_PASSWORD:-changeme}
      DB_DATABASE_NAME: immich
      REDIS_HOSTNAME: redis
    volumes: ["upload:/usr/src/app/upload"]
    depends_on: [db, redis]
  redis:
    image: redis:7-alpine
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: immich
      POSTGRES_USER: immich
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  upload:
  db_data:`,
	},
	{
		Slug: "paperless-ngx", Name: "Paperless-ngx", Category: "productivity",
		Description: "Document management system — scan, index, and archive papers",
		Tags:        []string{"documents", "ocr", "archive"}, Author: "Paperless-ngx", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
		ComposeYAML: `services:
  paperless:
    image: ghcr.io/paperless-ngx/paperless-ngx:latest
    ports: ["8000:8000"]
    environment:
      PAPERLESS_REDIS: redis://redis:6379
      PAPERLESS_DBHOST: db
      PAPERLESS_ADMIN_USER: ${ADMIN_USER:-admin}
      PAPERLESS_ADMIN_PASSWORD: ${ADMIN_PASSWORD:-changeme}
    volumes: ["data:/usr/src/paperless/data", "media:/usr/src/paperless/media"]
    depends_on: [db, redis]
  redis:
    image: redis:7-alpine
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: paperless
      POSTGRES_USER: paperless
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["pgdata:/var/lib/postgresql/data"]
volumes:
  data:
  media:
  pgdata:`,
	},
	{
		Slug: "hedgedoc", Name: "HedgeDoc", Category: "collaboration",
		Description: "Real-time collaborative markdown editor",
		Tags:        []string{"markdown", "wiki", "collaboration"}, Author: "HedgeDoc", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  hedgedoc:
    image: quay.io/hedgedoc/hedgedoc:latest
    ports: ["3000:3000"]
    environment:
      CMD_DB_URL: postgres://hedgedoc:${DB_PASSWORD:-changeme}@db:5432/hedgedoc
      CMD_DOMAIN: ${DOMAIN:-localhost}
      CMD_PROTOCOL_USESSL: "false"
    depends_on: [db]
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: hedgedoc
      POSTGRES_USER: hedgedoc
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	{
		Slug: "actual-budget", Name: "Actual Budget", Category: "finance",
		Description: "Privacy-focused personal finance and budgeting app",
		Tags:        []string{"budget", "finance", "money"}, Author: "Actual Budget", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
		ComposeYAML: `services:
  actual:
    image: actualbudget/actual-server:latest
    ports: ["5006:5006"]
    volumes: ["actual_data:/data"]
volumes:
  actual_data:`,
	},
	{
		Slug: "linkwarden", Name: "Linkwarden", Category: "productivity",
		Description: "Self-hosted bookmark manager with collaboration",
		Tags:        []string{"bookmarks", "links", "archive"}, Author: "Linkwarden", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  linkwarden:
    image: ghcr.io/linkwarden/linkwarden:latest
    ports: ["3000:3000"]
    environment:
      DATABASE_URL: postgres://linkwarden:${DB_PASSWORD:-changeme}@db:5432/linkwarden
      NEXTAUTH_SECRET: ${SECRET:-change-this}
      NEXTAUTH_URL: ${URL:-http://localhost:3000}
    depends_on: [db]
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: linkwarden
      POSTGRES_USER: linkwarden
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	// === CMS & BLOGGING ===
	{
		Slug: "drupal", Name: "Drupal", Category: "cms",
		Description: "Enterprise-grade open-source CMS",
		Tags:        []string{"cms", "php", "enterprise"}, Author: "Drupal", Version: "10",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024},
		ComposeYAML: `services:
  drupal:
    image: drupal:10-apache
    ports: ["80:80"]
    volumes: ["drupal_data:/var/www/html"]
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: drupal
      POSTGRES_USER: drupal
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  drupal_data:
  db_data:`,
	},
	{
		Slug: "strapi", Name: "Strapi", Category: "cms",
		Description: "Headless CMS for modern websites",
		Tags:        []string{"cms", "headless", "nodejs"}, Author: "Strapi", Version: "4",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  strapi:
    image: strapi/strapi:4
    ports: ["1337:1337"]
    environment:
      DATABASE_CLIENT: postgres
      DATABASE_HOST: db
      DATABASE_PORT: 5432
      DATABASE_NAME: strapi
      DATABASE_USERNAME: strapi
      DATABASE_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["strapi_data:/srv/app"]
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: strapi
      POSTGRES_USER: strapi
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  strapi_data:
  db_data:`,
	},
	{
		Slug: "payload", Name: "Payload CMS", Category: "cms",
		Description: "Code-first headless CMS built with TypeScript",
		Tags:        []string{"cms", "headless", "typescript"}, Author: "Payload", Version: "2",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 256},
		ComposeYAML: `services:
  payload:
    image: node:20-alpine
    ports: ["3000:3000"]
    environment:
      DATABASE_URI: mongodb://db:27017/payload
      PAYLOAD_SECRET: ${SECRET:-changeme}
    volumes: ["payload_data:/app"]
    working_dir: /app
    command: sh -c "npm install && npm run dev"
  db:
    image: mongo:7
    volumes: ["db_data:/data/db"]
volumes:
  payload_data:
  db_data:`,
	},
	{
		Slug: "ghostfolio", Name: "Ghostfolio", Category: "finance",
		Description: "Personal finance management and wealth tracking",
		Tags:        []string{"finance", "portfolio", "investing"}, Author: "Ghostfolio", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
		ComposeYAML: `services:
  ghostfolio:
    image: ghostfolio/ghostfolio:latest
    ports: ["3333:3333"]
    environment:
      DATABASE_URL: postgresql://ghostfolio:ghostfolio@db:5432/ghostfolio
      REDIS_HOST: redis
      JWT_SECRET_KEY: ${JWT_SECRET:-changeme}
    depends_on: [db, redis]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: ghostfolio
      POSTGRES_USER: ghostfolio
      POSTGRES_PASSWORD: ghostfolio
    volumes: ["db_data:/var/lib/postgresql/data"]
  redis:
    image: redis:7-alpine
    volumes: ["redis_data:/data"]
volumes:
  db_data:
  redis_data:`,
	},

	// === E-COMMERCE ===
	{
		Slug: "medusa", Name: "Medusa", Category: "ecommerce",
		Description: "Open-source Shopify alternative",
		Tags:        []string{"ecommerce", "shop", "nodejs"}, Author: "Medusa", Version: "2",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  medusa:
    image: ghcr.io/medusajs/medusa:latest
    ports: ["9000:9000"]
    environment:
      DATABASE_URL: postgres://medusa:medusa@db:5432/medusa
      REDIS_URL: redis://redis:6379
      JWT_SECRET: ${JWT_SECRET:-changeme}
    depends_on: [db, redis]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: medusa
      POSTGRES_USER: medusa
      POSTGRES_PASSWORD: medusa
    volumes: ["db_data:/var/lib/postgresql/data"]
  redis:
    image: redis:7-alpine
volumes:
  db_data:`,
	},
	{
		Slug: "prestashop", Name: "PrestaShop", Category: "ecommerce",
		Description: "Popular open-source e-commerce platform",
		Tags:        []string{"ecommerce", "shop", "php"}, Author: "PrestaShop", Version: "8",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 2048},
		ComposeYAML: `services:
  prestashop:
    image: prestashop/prestashop:8
    ports: ["80:80"]
    environment:
      DB_SERVER: db
      DB_NAME: prestashop
      DB_USER: prestashop
      DB_PASSWD: ${DB_PASSWORD:-changeme}
    volumes: ["ps_data:/var/www/html"]
    depends_on: [db]
  db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: prestashop
      MYSQL_USER: prestashop
      MYSQL_PASSWORD: ${DB_PASSWORD:-changeme}
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  ps_data:
  db_data:`,
	},
	{
		Slug: "sylius", Name: "Sylius", Category: "ecommerce",
		Description: "E-commerce platform for Symfony developers",
		Tags:        []string{"ecommerce", "shop", "symfony"}, Author: "Sylius", Version: "2",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  sylius:
    image: sylius/php:8.2-alpine
    ports: ["80:80"]
    environment:
      DATABASE_URL: mysql://sylius:sylius@db:3306/sylius
    volumes: ["sylius_data:/srv/sylius"]
    depends_on: [db]
  db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: sylius
      MYSQL_USER: sylius
      MYSQL_PASSWORD: sylius
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  sylius_data:
  db_data:`,
	},

	// === MONITORING & OBSERVABILITY ===
	{
		Slug: "grafana", Name: "Grafana", Category: "monitoring",
		Description: "Observability and data visualization platform",
		Tags:        []string{"monitoring", "metrics", "dashboard"}, Author: "Grafana Labs", Version: "11",
		Verified: true, Featured: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  grafana:
    image: grafana/grafana:11
    ports: ["3000:3000"]
    environment:
      GF_SECURITY_ADMIN_PASSWORD: ${PASSWORD:-admin}
    volumes: ["grafana_data:/var/lib/grafana"]
volumes:
  grafana_data:`,
	},
	{
		Slug: "prometheus", Name: "Prometheus", Category: "monitoring",
		Description: "Time-series database and monitoring system",
		Tags:        []string{"monitoring", "metrics", "alerting"}, Author: "CNCF", Version: "2",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  prometheus:
    image: prom/prometheus:v2.54.0
    ports: ["9090:9090"]
    volumes:
      - prometheus_data:/prometheus
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    command: --config.file=/etc/prometheus/prometheus.yml
volumes:
  prometheus_data:`,
	},
	{
		Slug: "loki", Name: "Loki", Category: "monitoring",
		Description: "Log aggregation system by Grafana Labs",
		Tags:        []string{"logging", "monitoring", "grafana"}, Author: "Grafana Labs", Version: "3",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 1024},
		ComposeYAML: `services:
  loki:
    image: grafana/loki:3
    ports: ["3100:3100"]
    volumes: ["loki_data:/loki"]
    command: -config.file=/etc/loki/local-config.yaml
volumes:
  loki_data:`,
	},
	{
		Slug: "tempo", Name: "Tempo", Category: "monitoring",
		Description: "Distributed tracing backend by Grafana Labs",
		Tags:        []string{"tracing", "monitoring", "grafana"}, Author: "Grafana Labs", Version: "2",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 1024},
		ComposeYAML: `services:
  tempo:
    image: grafana/tempo:2
    ports: ["3200:3200", "4317:4317", "4318:4318"]
    volumes: ["tempo_data:/var/tempo"]
    command: -config.file=/etc/tempo.yaml
volumes:
  tempo_data:`,
	},
	{
		Slug: "jaeger", Name: "Jaeger", Category: "monitoring",
		Description: "Distributed tracing platform",
		Tags:        []string{"tracing", "monitoring", "microservices"}, Author: "CNCF", Version: "1",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
		ComposeYAML: `services:
  jaeger:
    image: jaegertracing/all-in-one:1
    ports:
      - "16686:16686"
      - "4317:4317"
      - "4318:4318"
    environment:
      COLLECTOR_OTLP_ENABLED: "true"
volumes: {}`,
	},
	{
		Slug: "alertmanager", Name: "Alertmanager", Category: "monitoring",
		Description: "Prometheus alert management",
		Tags:        []string{"alerting", "monitoring", "prometheus"}, Author: "Prometheus", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 64, DiskMB: 64},
		ComposeYAML: `services:
  alertmanager:
    image: prom/alertmanager:latest
    ports: ["9093:9093"]
    volumes: ["alertmanager_data:/alertmanager"]
volumes:
  alertmanager_data:`,
	},
	{
		Slug: "cadvisor", Name: "cAdvisor", Category: "monitoring",
		Description: "Container resource monitoring",
		Tags:        []string{"containers", "monitoring", "metrics"}, Author: "Google", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 128, DiskMB: 64},
		ComposeYAML: `services:
  cadvisor:
    image: gcr.io/cadvisor/cadvisor:latest
    ports: ["8080:8080"]
    volumes:
      - /:/rootfs:ro
      - /var/run:/var/run:ro
      - /sys:/sys:ro
      - /var/lib/docker/:/var/lib/docker:ro
volumes: {}`,
	},

	// === COMMUNICATION ===
	{
		Slug: "matrix-synapse", Name: "Matrix Synapse", Category: "communication",
		Description: "Decentralized messaging server",
		Tags:        []string{"chat", "matrix", "federated"}, Author: "Matrix.org", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024},
		ComposeYAML: `services:
  synapse:
    image: matrixorg/synapse:latest
    ports: ["8008:8008"]
    environment:
      SYNAPSE_SERVER_NAME: ${SERVER_NAME:-localhost}
      SYNAPSE_REPORT_STATS: "no"
    volumes: ["synapse_data:/data"]
volumes:
  synapse_data:`,
	},
	{
		Slug: "matrix-element", Name: "Element Web", Category: "communication",
		Description: "Matrix web client",
		Tags:        []string{"chat", "matrix", "web"}, Author: "Element", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 128, DiskMB: 64},
		ComposeYAML: `services:
  element:
    image: vectorim/element-web:latest
    ports: ["80:80"]
    volumes:
      - ./element-config.json:/app/config.json:ro
volumes: {}`,
	},
	{
		Slug: "rocketchat", Name: "Rocket.Chat", Category: "communication",
		Description: "Open-source team chat platform",
		Tags:        []string{"chat", "team", "slack"}, Author: "Rocket.Chat", Version: "6",
		Verified: true, MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
		ComposeYAML: `services:
  rocketchat:
    image: rocket.chat:6
    ports: ["3000:3000"]
    environment:
      MONGO_URL: mongodb://db:27017/rocketchat
      MONGO_OPLOG_URL: mongodb://db:27017/local
    depends_on: [db]
  db:
    image: mongo:7
    command: mongod --replSet rs0
    volumes: ["db_data:/data/db"]
volumes:
  db_data:`,
	},
	{
		Slug: "mattermost", Name: "Mattermost", Category: "communication",
		Description: "Enterprise messaging platform",
		Tags:        []string{"chat", "team", "enterprise"}, Author: "Mattermost", Version: "9",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 2048},
		ComposeYAML: `services:
  mattermost:
    image: mattermost/mattermost-team-edition:9
    ports: ["8065:8065"]
    environment:
      MM_SQLSETTINGS_DRIVERNAME: postgres
      MM_SQLSETTINGS_DATASOURCE: postgres://mmuser:mmuser@db:5432/mattermost
    volumes: ["mm_data:/mattermost/data"]
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: mattermost
      POSTGRES_USER: mmuser
      POSTGRES_PASSWORD: mmuser
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  mm_data:
  db_data:`,
	},
	{
		Slug: "zulip", Name: "Zulip", Category: "communication",
		Description: "Team chat with threaded conversations",
		Tags:        []string{"chat", "team", "threads"}, Author: "Zulip", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
		ComposeYAML: `services:
  zulip:
    image: zulip/docker-zulip:latest
    ports: ["80:80", "443:443"]
    environment:
      ZULIP_AUTH_BACKENDS: EmailAuthBackend
      SETTING_MEMCACHED_LOCATION: memcached:11211
      SETTING_RABBITMQ_HOST: rabbitmq
      SETTING_REDIS_HOST: redis
    depends_on: [db, memcached, rabbitmq, redis]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: zulip
      POSTGRES_USER: zulip
      POSTGRES_PASSWORD: zulip
    volumes: ["db_data:/var/lib/postgresql/data"]
  memcached:
    image: memcached:alpine
  rabbitmq:
    image: rabbitmq:3-alpine
  redis:
    image: redis:7-alpine
volumes:
  db_data:`,
	},

	// === MEDIA & ENTERTAINMENT ===
	{
		Slug: "jellyfin", Name: "Jellyfin", Category: "media",
		Description: "Free software media system",
		Tags:        []string{"media", "streaming", "movies"}, Author: "Jellyfin", Version: "latest",
		Verified: true, Featured: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 2048},
		ComposeYAML: `services:
  jellyfin:
    image: jellyfin/jellyfin:latest
    ports: ["8096:8096"]
    volumes:
      - jellyfin_config:/config
      - jellyfin_cache:/cache
      - ${MEDIA_PATH:-/media}:/media
volumes:
  jellyfin_config:
  jellyfin_cache:`,
	},
	{
		Slug: "navidrome", Name: "Navidrome", Category: "media",
		Description: "Music server and streamer",
		Tags:        []string{"music", "streaming", "subsonic"}, Author: "Navidrome", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 128, DiskMB: 512},
		ComposeYAML: `services:
  navidrome:
    image: deluan/navidrome:latest
    ports: ["4533:4533"]
    environment:
      ND_SCANSCHEDULE: "1h"
    volumes:
      - navidrome_data:/data
      - ${MUSIC_PATH:-/music}:/music:ro
volumes:
  navidrome_data:`,
	},
	{
		Slug: "immich", Name: "Immich", Category: "media",
		Description: "Self-hosted photo and video backup",
		Tags:        []string{"photos", "backup", "google-photos"}, Author: "Immich", Version: "latest",
		Verified: true, Featured: true, MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 5120},
		ComposeYAML: `services:
  immich-server:
    image: ghcr.io/immich-app/immich-server:latest
    ports: ["2283:2283"]
    environment:
      DB_HOSTNAME: db
      DB_USERNAME: immich
      DB_PASSWORD: ${DB_PASSWORD:-immich}
      DB_DATABASE_NAME: immich
      REDIS_HOSTNAME: redis
    volumes:
      - immich_upload:/usr/src/app/upload
      - ${PHOTOS_PATH:-/photos}:/usr/src/app/external
    depends_on: [db, redis]
  db:
    image: tensorchord/pgvecto-rs:pg16
    environment:
      POSTGRES_DB: immich
      POSTGRES_USER: immich
      POSTGRES_PASSWORD: ${DB_PASSWORD:-immich}
    volumes: ["db_data:/var/lib/postgresql/data"]
  redis:
    image: redis:7-alpine
volumes:
  immich_upload:
  db_data:`,
	},
	{
		Slug: "photoprism", Name: "PhotoPrism", Category: "media",
		Description: "AI-powered photos app",
		Tags:        []string{"photos", "ai", "gallery"}, Author: "PhotoPrism", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 2048, DiskMB: 5120},
		ComposeYAML: `services:
  photoprism:
    image: photoprism/photoprism:latest
    ports: ["2342:2342"]
    environment:
      PHOTOPRISM_ADMIN_PASSWORD: ${PASSWORD:-admin}
      PHOTOPRISM_DATABASE_DRIVER: mysql
      PHOTOPRISM_DATABASE_DSN: photoprism:photoprism@tcp(db)/photoprism
    volumes:
      - photoprism_originals:/photoprism/originals
      - photoprism_storage:/photoprism/storage
    depends_on: [db]
  db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: photoprism
      MYSQL_USER: photoprism
      MYSQL_PASSWORD: photoprism
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  photoprism_originals:
  photoprism_storage:
  db_data:`,
	},
	{
		Slug: "audiobookshelf", Name: "Audiobookshelf", Category: "media",
		Description: "Audiobook and podcast server",
		Tags:        []string{"audiobooks", "podcasts", "media"}, Author: "advplyr", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 1024},
		ComposeYAML: `services:
  audiobookshelf:
    image: ghcr.io/advplyr/audiobookshelf:latest
    ports: ["13378:80"]
    volumes:
      - audiobooks:/audiobooks
      - podcasts:/podcasts
      - config:/config
      - metadata:/metadata
volumes:
  audiobooks:
  podcasts:
  config:
  metadata:`,
	},

	// === PRODUCTIVITY & DOCS ===
	{
		Slug: "paperless-ngx", Name: "Paperless-NGX", Category: "productivity",
		Description: "Document management system",
		Tags:        []string{"documents", "ocr", "paperless"}, Author: "Paperless-NGX", Version: "latest",
		Verified: true, Featured: true, MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
		ComposeYAML: `services:
  paperless:
    image: ghcr.io/paperless-ngx/paperless-ngx:latest
    ports: ["8000:8000"]
    environment:
      PAPERLESS_DBHOST: db
      PAPERLESS_DBUSER: paperless
      PAPERLESS_DBPASS: ${DB_PASSWORD:-paperless}
      PAPERLESS_DBNAME: paperless
      PAPERLESS_SECRET_KEY: ${SECRET_KEY:-changeme}
      PAPERLESS_URL: ${URL:-http://localhost:8000}
    volumes:
      - paperless_data:/usr/src/paperless/data
      - paperless_media:/usr/src/paperless/media
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: paperless
      POSTGRES_USER: paperless
      POSTGRES_PASSWORD: ${DB_PASSWORD:-paperless}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  paperless_data:
  paperless_media:
  db_data:`,
	},
	{
		Slug: "bookstack", Name: "BookStack", Category: "productivity",
		Description: "Wiki and documentation platform",
		Tags:        []string{"wiki", "docs", "knowledge"}, Author: "BookStack", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  bookstack:
    image: lscr.io/linuxserver/bookstack:latest
    ports: ["80:80"]
    environment:
      DB_HOST: db
      DB_DATABASE: bookstack
      DB_USERNAME: bookstack
      DB_PASSWORD: ${DB_PASSWORD:-bookstack}
      APP_URL: ${APP_URL:-http://localhost}
    volumes: ["bookstack_data:/config"]
    depends_on: [db]
  db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: bookstack
      MYSQL_USER: bookstack
      MYSQL_PASSWORD: ${DB_PASSWORD:-bookstack}
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  bookstack_data:
  db_data:`,
	},
	{
		Slug: "wikijs", Name: "Wiki.js", Category: "productivity",
		Description: "Modern wiki application",
		Tags:        []string{"wiki", "docs", "markdown"}, Author: "Requarks", Version: "2",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  wikijs:
    image: ghcr.io/requarks/wiki:2
    ports: ["3000:3000"]
    environment:
      DB_TYPE: postgres
      DB_HOST: db
      DB_PORT: 5432
      DB_USER: wiki
      DB_PASS: ${DB_PASSWORD:-wiki}
      DB_NAME: wiki
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: wiki
      POSTGRES_USER: wiki
      POSTGRES_PASSWORD: ${DB_PASSWORD:-wiki}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	{
		Slug: "outline", Name: "Outline", Category: "productivity",
		Description: "Team knowledge base and wiki",
		Tags:        []string{"wiki", "docs", "notion"}, Author: "Outline", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  outline:
    image: outlinewiki/outline:latest
    ports: ["3000:3000"]
    environment:
      DATABASE_URL: postgres://outline:outline@db:5432/outline
      REDIS_URL: redis://redis:6379
      SECRET_KEY: ${SECRET_KEY:-changeme}
      UTILS_SECRET: ${UTILS_SECRET:-changeme}
      URL: ${URL:-http://localhost:3000}
    depends_on: [db, redis]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: outline
      POSTGRES_USER: outline
      POSTGRES_PASSWORD: outline
    volumes: ["db_data:/var/lib/postgresql/data"]
  redis:
    image: redis:7-alpine
volumes:
  db_data:`,
	},
	{
		Slug: "nocodb", Name: "NocoDB", Category: "productivity",
		Description: "Airtable alternative with spreadsheets",
		Tags:        []string{"database", "spreadsheet", "airtable"}, Author: "NocoDB", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  nocodb:
    image: nocodb/nocodb:latest
    ports: ["8080:8080"]
    environment:
      NC_DB: "pg://db?u=nocodb&p=nocodb&d=nocodb"
      NC_PUBLIC_URL: ${URL:-http://localhost:8080}
    volumes: ["nocodb_data:/usr/app/data"]
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: nocodb
      POSTGRES_USER: nocodb
      POSTGRES_PASSWORD: nocodb
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  nocodb_data:
  db_data:`,
	},
	{
		Slug: "baserow", Name: "Baserow", Category: "productivity",
		Description: "No-code database and Airtable alternative",
		Tags:        []string{"database", "no-code", "airtable"}, Author: "Baserow", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  baserow:
    image: baserow/baserow:latest
    ports: ["80:80"]
    environment:
      BASEROW_PUBLIC_URL: ${URL:-http://localhost}
      DATABASE_HOST: db
      DATABASE_USER: baserow
      DATABASE_PASSWORD: ${DB_PASSWORD:-baserow}
      DATABASE_NAME: baserow
    volumes: ["baserow_data:/baserow/data"]
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: baserow
      POSTGRES_USER: baserow
      POSTGRES_PASSWORD: ${DB_PASSWORD:-baserow}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  baserow_data:
  db_data:`,
	},

	// === SECURITY & AUTH ===
	{
		Slug: "keycloak", Name: "Keycloak", Category: "security",
		Description: "Identity and access management",
		Tags:        []string{"sso", "oauth", "identity"}, Author: "Red Hat", Version: "25",
		Verified: true, Featured: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  keycloak:
    image: quay.io/keycloak/keycloak:25
    ports: ["8080:8080"]
    environment:
      KC_DB: postgres
      KC_DB_URL_HOST: db
      KC_DB_USERNAME: keycloak
      KC_DB_PASSWORD: ${DB_PASSWORD:-keycloak}
      KC_HOSTNAME: ${HOSTNAME:-localhost}
      KEYCLOAK_ADMIN: admin
      KEYCLOAK_ADMIN_PASSWORD: ${ADMIN_PASSWORD:-admin}
    command: start-dev
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: keycloak
      POSTGRES_USER: keycloak
      POSTGRES_PASSWORD: ${DB_PASSWORD:-keycloak}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	{
		Slug: "authentik", Name: "Authentik", Category: "security",
		Description: "Open-source identity provider",
		Tags:        []string{"sso", "oauth", "auth"}, Author: "goauthentik", Version: "2024",
		Verified: true, MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 1024},
		ComposeYAML: `services:
  authentik-server:
    image: ghcr.io/goauthentik/server:latest
    ports: ["9000:9000", "9443:9443"]
    environment:
      AUTHENTIK_SECRET_KEY: ${SECRET_KEY:-changeme}
      AUTHENTIK_REDIS__HOST: redis
      AUTHENTIK_POSTGRESQL__HOST: db
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: ${DB_PASSWORD:-authentik}
      AUTHENTIK_POSTGRESQL__NAME: authentik
    depends_on: [db, redis]
  authentik-worker:
    image: ghcr.io/goauthentik/server:latest
    command: worker
    environment:
      AUTHENTIK_SECRET_KEY: ${SECRET_KEY:-changeme}
      AUTHENTIK_REDIS__HOST: redis
      AUTHENTIK_POSTGRESQL__HOST: db
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: ${DB_PASSWORD:-authentik}
      AUTHENTIK_POSTGRESQL__NAME: authentik
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: authentik
      POSTGRES_USER: authentik
      POSTGRES_PASSWORD: ${DB_PASSWORD:-authentik}
    volumes: ["db_data:/var/lib/postgresql/data"]
  redis:
    image: redis:7-alpine
volumes:
  db_data:`,
	},
	{
		Slug: "authelia", Name: "Authelia", Category: "security",
		Description: "SSO and 2FA portal",
		Tags:        []string{"sso", "2fa", "auth"}, Author: "Authelia", Version: "4",
		Verified: true, MinResources: ResourceReq{MemoryMB: 128, DiskMB: 128},
		ComposeYAML: `services:
  authelia:
    image: authelia/authelia:4
    ports: ["9091:9091"]
    environment:
      AUTHELIA_STORAGE_POSTGRES_HOST: db
      AUTHELIA_STORAGE_POSTGRES_USERNAME: authelia
      AUTHELIA_STORAGE_POSTGRES_PASSWORD: ${DB_PASSWORD:-authelia}
      AUTHELIA_STORAGE_POSTGRES_DATABASE: authelia
      AUTHELIA_STORAGE_ENCRYPTION_KEY: ${ENCRYPTION_KEY:-changeme}
      AUTHELIA_JWT_SECRET: ${JWT_SECRET:-changeme}
    volumes: ["authelia_config:/config"]
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: authelia
      POSTGRES_USER: authelia
      POSTGRES_PASSWORD: ${DB_PASSWORD:-authelia}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  authelia_config:
  db_data:`,
	},
	{
		Slug: "portainer", Name: "Portainer", Category: "security",
		Description: "Container management platform",
		Tags:        []string{"docker", "containers", "management"}, Author: "Portainer", Version: "2",
		Verified: true, Featured: true, MinResources: ResourceReq{MemoryMB: 128, DiskMB: 256},
		ComposeYAML: `services:
  portainer:
    image: portainer/portainer-ce:2
    ports: ["9443:9443", "9000:9000"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - portainer_data:/data
volumes:
  portainer_data:`,
	},

	// === AI & ML ===
	{
		Slug: "open-webui", Name: "Open WebUI", Category: "ai",
		Description: "ChatGPT-style UI for Ollama",
		Tags:        []string{"ai", "llm", "chat"}, Author: "Open WebUI", Version: "latest",
		Verified: true, Featured: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  open-webui:
    image: ghcr.io/open-webui/open-webui:main
    ports: ["3000:8080"]
    environment:
      OLLAMA_BASE_URL: ${OLLAMA_URL:-http://ollama:11434}
    volumes: ["openwebui_data:/app/backend/data"]
volumes:
  openwebui_data:`,
	},
	{
		Slug: "localai", Name: "LocalAI", Category: "ai",
		Description: "OpenAI-compatible local LLM server",
		Tags:        []string{"ai", "llm", "openai"}, Author: "LocalAI", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 4096, DiskMB: 10240},
		ComposeYAML: `services:
  localai:
    image: localai/localai:latest
    ports: ["8080:8080"]
    environment:
      MODELS_PATH: /models
    volumes:
      - localai_models:/models
      - localai_data:/build
volumes:
  localai_models:
  localai_data:`,
	},
	{
		Slug: "stable-diffusion", Name: "Stable Diffusion WebUI", Category: "ai",
		Description: "AI image generation",
		Tags:        []string{"ai", "image", "diffusion"}, Author: "AUTOMATIC1111", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 8192, DiskMB: 20480},
		ComposeYAML: `services:
  sd-webui:
    image: universallm/stable-diffusion-webui:latest
    ports: ["7860:7860"]
    environment:
      COMMANDLINE_ARGS: --listen --xformers
    volumes:
      - sd_models:/models
      - sd_outputs:/outputs
volumes:
  sd_models:
  sd_outputs:`,
	},
	{
		Slug: "text-generation-webui", Name: "Text Generation WebUI", Category: "ai",
		Description: "Gradio UI for text generation",
		Tags:        []string{"ai", "llm", "text"}, Author: "oobabooga", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 8192, DiskMB: 20480},
		ComposeYAML: `services:
  textgen:
    image: ghcr.io/oobabooga/text-generation-webui:latest
    ports: ["7860:7860"]
    environment:
      CLI_ARGS: --listen
    volumes:
      - textgen_models:/app/models
      - textgen_data:/app/data
volumes:
  textgen_models:
  textgen_data:`,
	},
	{
		Slug: "searxng", Name: "SearXNG", Category: "ai",
		Description: "Privacy-respecting metasearch engine",
		Tags:        []string{"search", "privacy", "metasearch"}, Author: "SearXNG", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 128},
		ComposeYAML: `services:
  searxng:
    image: searxng/searxng:latest
    ports: ["8080:8080"]
    environment:
      SEARXNG_BASE_URL: ${URL:-http://localhost:8080}
    volumes: ["searxng_data:/etc/searxng"]
volumes:
  searxng_data:`,
	},

	// === AUTOMATION ===
	{
		Slug: "nodered", Name: "Node-RED", Category: "automation",
		Description: "Flow-based programming for IoT",
		Tags:        []string{"automation", "iot", "flow"}, Author: "Node-RED", Version: "3",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
		ComposeYAML: `services:
  nodered:
    image: nodered/node-red:3
    ports: ["1880:1880"]
    volumes: ["nodered_data:/data"]
volumes:
  nodered_data:`,
	},
	{
		Slug: "activepieces", Name: "ActivePieces", Category: "automation",
		Description: "No-code workflow automation",
		Tags:        []string{"automation", "workflow", "no-code"}, Author: "ActivePieces", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  activepieces:
    image: activepieces/activepieces:latest
    ports: ["8080:80"]
    environment:
      AP_ENGINE_EXECUTABLE_PATH: dist/packages/engine/main.js
      AP_POSTGRES_HOST: db
      AP_POSTGRES_USER: activepieces
      AP_POSTGRES_PASSWORD: ${DB_PASSWORD:-activepieces}
      AP_POSTGRES_DATABASE: activepieces
      AP_JWT_SECRET: ${JWT_SECRET:-changeme}
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: activepieces
      POSTGRES_USER: activepieces
      POSTGRES_PASSWORD: ${DB_PASSWORD:-activepieces}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	{
		Slug: "huginn", Name: "Huginn", Category: "automation",
		Description: "Private IFTTT/Zapier alternative",
		Tags:        []string{"automation", "agents", "scraping"}, Author: "Huginn", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  huginn:
    image: huginn/huginn:latest
    ports: ["3000:3000"]
    environment:
      DATABASE_ADAPTER: postgresql
      DATABASE_HOST: db
      DATABASE_USERNAME: huginn
      DATABASE_PASSWORD: ${DB_PASSWORD:-huginn}
      DATABASE_NAME: huginn
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: huginn
      POSTGRES_USER: huginn
      POSTGRES_PASSWORD: ${DB_PASSWORD:-huginn}
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	{
		Slug: "trigger", Name: "Trigger.dev", Category: "automation",
		Description: "Background jobs and workflows",
		Tags:        []string{"automation", "jobs", "workflows"}, Author: "Trigger.dev", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  trigger:
    image: ghcr.io/triggerdotdev/trigger.dev:latest
    ports: ["3030:3030"]
    environment:
      DATABASE_URL: postgres://trigger:trigger@db:5432/trigger
      REDIS_URL: redis://redis:6379
      SESSION_SECRET: ${SECRET:-changeme}
      MAGIC_LINK_SECRET: ${MAGIC_SECRET:-changeme}
    depends_on: [db, redis]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: trigger
      POSTGRES_USER: trigger
      POSTGRES_PASSWORD: trigger
    volumes: ["db_data:/var/lib/postgresql/data"]
  redis:
    image: redis:7-alpine
volumes:
  db_data:`,
	},

	// === DATABASE TOOLS ===
	{
		Slug: "pgadmin", Name: "pgAdmin 4", Category: "devtools",
		Description: "PostgreSQL administration tool",
		Tags:        []string{"database", "postgres", "admin"}, Author: "pgAdmin", Version: "8",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
		ComposeYAML: `services:
  pgadmin:
    image: dpage/pgadmin4:8
    ports: ["5050:80"]
    environment:
      PGADMIN_DEFAULT_EMAIL: ${EMAIL:-admin@localhost}
      PGADMIN_DEFAULT_PASSWORD: ${PASSWORD:-admin}
    volumes: ["pgadmin_data:/var/lib/pgadmin"]
volumes:
  pgadmin_data:`,
	},
	{
		Slug: "mongo-express", Name: "Mongo Express", Category: "devtools",
		Description: "MongoDB web interface",
		Tags:        []string{"database", "mongodb", "admin"}, Author: "Mongo Express", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 128, DiskMB: 64},
		ComposeYAML: `services:
  mongo-express:
    image: mongo-express:latest
    ports: ["8081:8081"]
    environment:
      ME_CONFIG_MONGODB_URL: ${MONGO_URL:-mongodb://mongo:27017}
      ME_CONFIG_BASICAUTH_USERNAME: ${USER:-admin}
      ME_CONFIG_BASICAUTH_PASSWORD: ${PASSWORD:-admin}
volumes: {}`,
	},
	{
		Slug: "redis-commander", Name: "Redis Commander", Category: "devtools",
		Description: "Redis web management tool",
		Tags:        []string{"database", "redis", "admin"}, Author: "Redis Commander", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 64, DiskMB: 32},
		ComposeYAML: `services:
  redis-commander:
    image: rediscommander/redis-commander:latest
    ports: ["8081:8081"]
    environment:
      REDIS_HOSTS: local:${REDIS_HOST:-redis}:6379
volumes: {}`,
	},

	// === ANALYTICS ===
	{
		Slug: "umami", Name: "Umami", Category: "analytics",
		Description: "Simple, privacy-focused analytics",
		Tags:        []string{"analytics", "privacy", "web"}, Author: "Umami", Version: "2",
		Verified: true, MinResources: ResourceReq{MemoryMB: 128, DiskMB: 256},
		ComposeYAML: `services:
  umami:
    image: ghcr.io/umami-software/umami:postgresql-latest
    ports: ["3000:3000"]
    environment:
      DATABASE_URL: postgresql://umami:umami@db:5432/umami
      HASH_SALT: ${SALT:-changeme}
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: umami
      POSTGRES_USER: umami
      POSTGRES_PASSWORD: umami
    volumes: ["db_data:/var/lib/postgresql/data"]
volumes:
  db_data:`,
	},
	{
		Slug: "matomo", Name: "Matomo", Category: "analytics",
		Description: "Google Analytics alternative",
		Tags:        []string{"analytics", "privacy", "web"}, Author: "Matomo", Version: "5",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 512},
		ComposeYAML: `services:
  matomo:
    image: matomo:5
    ports: ["80:80"]
    environment:
      MATOMO_DATABASE_HOST: db
      MATOMO_DATABASE_USERNAME: matomo
      MATOMO_DATABASE_PASSWORD: ${DB_PASSWORD:-matomo}
      MATOMO_DATABASE_DBNAME: matomo
    volumes: ["matomo_data:/var/www/html"]
    depends_on: [db]
  db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: matomo
      MYSQL_USER: matomo
      MYSQL_PASSWORD: ${DB_PASSWORD:-matomo}
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  matomo_data:
  db_data:`,
	},

	// === STORAGE & FILES ===
	{
		Slug: "seafile", Name: "Seafile", Category: "storage",
		Description: "Cloud storage and file sync",
		Tags:        []string{"storage", "sync", "cloud"}, Author: "Seafile", Version: "11",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 2048},
		ComposeYAML: `services:
  seafile:
    image: seafileltd/seafile-mc:11
    ports: ["80:80"]
    environment:
      DB_HOST: db
      DB_USER: seafile
      DB_PASSWORD: ${DB_PASSWORD:-seafile}
      TIME_ZONE: Etc/UTC
    volumes: ["seafile_data:/shared"]
    depends_on: [db, memcached]
  db:
    image: mariadb:11
    environment:
      MYSQL_DATABASE: seafile
      MYSQL_USER: seafile
      MYSQL_PASSWORD: ${DB_PASSWORD:-seafile}
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
  memcached:
    image: memcached:alpine
volumes:
  seafile_data:
  db_data:`,
	},
	{
		Slug: "filebrowser", Name: "File Browser", Category: "storage",
		Description: "Web file manager",
		Tags:        []string{"files", "storage", "web"}, Author: "File Browser", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 64, DiskMB: 128},
		ComposeYAML: `services:
  filebrowser:
    image: filebrowser/filebrowser:latest
    ports: ["80:80"]
    volumes:
      - ${FILES_PATH:-/srv}:/srv
      - filebrowser_data:/database
      - filebrowser_settings:/config
volumes:
  filebrowser_data:
  filebrowser_settings:`,
	},
	{
		Slug: "projectsend", Name: "ProjectSend", Category: "storage",
		Description: "File sharing for clients",
		Tags:        []string{"files", "sharing", "clients"}, Author: "ProjectSend", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 1024},
		ComposeYAML: `services:
  projectsend:
    image: linuxserver/projectsend:latest
    ports: ["80:80"]
    environment:
      DB_TYPE: mysql
      DB_HOST: db
      DB_USER: projectsend
      DB_PASSWORD: ${DB_PASSWORD:-projectsend}
      DB_NAME: projectsend
    volumes:
      - projectsend_data:/config
      - projectsend_files:/data
    depends_on: [db]
  db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: projectsend
      MYSQL_USER: projectsend
      MYSQL_PASSWORD: ${DB_PASSWORD:-projectsend}
      MYSQL_ROOT_PASSWORD: ${DB_ROOT_PASSWORD:-rootpass}
    volumes: ["db_data:/var/lib/mysql"]
volumes:
  projectsend_data:
  projectsend_files:
  db_data:`,
	},

	// === DEV TOOLS ===
	{
		Slug: "gitlab", Name: "GitLab CE", Category: "devtools",
		Description: "DevOps platform",
		Tags:        []string{"git", "ci", "devops"}, Author: "GitLab", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 4096, DiskMB: 10240},
		ComposeYAML: `services:
  gitlab:
    image: gitlab/gitlab-ce:latest
    ports: ["80:80", "443:443", "22:22"]
    environment:
      GITLAB_OMNIBUS_CONFIG: |
        external_url '${URL:-http://localhost}'
        gitlab_rails['gitlab_shell_ssh_port'] = 22
    volumes:
      - gitlab_config:/etc/gitlab
      - gitlab_logs:/var/log/gitlab
      - gitlab_data:/var/opt/gitlab
volumes:
  gitlab_config:
  gitlab_logs:
  gitlab_data:`,
	},
	{
		Slug: "gogs", Name: "Gogs", Category: "devtools",
		Description: "Lightweight Git service",
		Tags:        []string{"git", "scm", "lightweight"}, Author: "Gogs", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 128, DiskMB: 256},
		ComposeYAML: `services:
  gogs:
    image: gogs/gogs:latest
    ports: ["3000:3000", "22:22"]
    volumes: ["gogs_data:/data"]
volumes:
  gogs_data:`,
	},
	{
		Slug: "drone", Name: "Drone CI", Category: "devtools",
		Description: "Container-native CI/CD",
		Tags:        []string{"ci", "cd", "docker"}, Author: "Drone", Version: "2",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
		ComposeYAML: `services:
  drone:
    image: drone/drone:2
    ports: ["80:80"]
    environment:
      DRONE_SERVER_HOST: ${HOST:-localhost}
      DRONE_SERVER_PROTO: ${PROTO:-http}
      DRONE_RPC_SECRET: ${SECRET:-changeme}
      DRONE_GITHUB_CLIENT_ID: ${GITHUB_CLIENT_ID:-}
      DRONE_GITHUB_CLIENT_SECRET: ${GITHUB_CLIENT_SECRET:-}
    volumes: ["drone_data:/data"]
volumes:
  drone_data:`,
	},
	{
		Slug: "woodpecker", Name: "Woodpecker CI", Category: "devtools",
		Description: "Community fork of Drone CI",
		Tags:        []string{"ci", "cd", "docker"}, Author: "Woodpecker", Version: "2",
		Verified: true, MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
		ComposeYAML: `services:
  woodpecker-server:
    image: woodpeckerci/woodpecker-server:latest
    ports: ["8000:8000"]
    environment:
      WOODPECKER_HOST: ${HOST:-http://localhost:8000}
      WOODPECKER_AGENT_SECRET: ${SECRET:-changeme}
      WOODPECKER_GITHUB_CLIENT: ${GITHUB_CLIENT:-}
      WOODPECKER_GITHUB_SECRET: ${GITHUB_SECRET:-}
    volumes: ["woodpecker_data:/var/lib/woodpecker"]
  woodpecker-agent:
    image: woodpeckerci/woodpecker-agent:latest
    environment:
      WOODPECKER_SERVER: woodpecker-server:9000
      WOODPECKER_AGENT_SECRET: ${SECRET:-changeme}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
volumes:
  woodpecker_data:`,
	},

	// === MISC ===
	{
		Slug: "it-tools", Name: "IT Tools", Category: "devtools",
		Description: "Useful tools for developers",
		Tags:        []string{"tools", "utilities", "dev"}, Author: "Corentin Thomasset", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 64, DiskMB: 32},
		ComposeYAML: `services:
  it-tools:
    image: corentinth/it-tools:latest
    ports: ["80:80"]
volumes: {}`,
	},
	{
		Slug: "stirling-pdf", Name: "Stirling-PDF", Category: "productivity",
		Description: "PDF manipulation toolkit",
		Tags:        []string{"pdf", "tools", "documents"}, Author: "Stirling-Tools", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  stirling-pdf:
    image: frooodle/s-pdf:latest
    ports: ["8080:8080"]
    volumes:
      - stirling_data:/usr/share/tessdata
      - stirling_configs:/configs
volumes:
  stirling_data:
  stirling_configs:`,
	},
	{
		Slug: "actual-budget", Name: "Actual Budget", Category: "finance",
		Description: "Personal finance app",
		Tags:        []string{"finance", "budget", "money"}, Author: "Actual", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 128, DiskMB: 256},
		ComposeYAML: `services:
  actual:
    image: actualbudget/actual-server:latest
    ports: ["5006:5006"]
    volumes: ["actual_data:/data"]
volumes:
  actual_data:`,
	},
	{
		Slug: "homeassistant", Name: "Home Assistant", Category: "iot",
		Description: "Smart home automation",
		Tags:        []string{"iot", "smart-home", "automation"}, Author: "Home Assistant", Version: "latest",
		Verified: true, Featured: true, MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024},
		ComposeYAML: `services:
  homeassistant:
    image: ghcr.io/home-assistant/home-assistant:stable
    ports: ["8123:8123"]
    volumes:
      - ha_config:/config
      - /etc/localtime:/etc/localtime:ro
    privileged: true
volumes:
  ha_config:`,
	},
	{
		Slug: "penpot", Name: "Penpot", Category: "design",
		Description: "Design and prototyping platform",
		Tags:        []string{"design", "figma", "prototype"}, Author: "Penpot", Version: "latest",
		Verified: true, MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 1024},
		ComposeYAML: `services:
  penpot-frontend:
    image: penpotapp/frontend:latest
    ports: ["9001:80"]
    environment:
      PENPOT_FLAGS: enable-registration enable-login
  penpot-backend:
    image: penpotapp/backend:latest
    environment:
      PENPOT_FLAGS: enable-registration enable-login
      PENPOT_DATABASE_URI: postgresql://penpot:penpot@postgres/penpot
      PENPOT_REDIS_URI: redis://redis/0
    depends_on: [postgres, redis]
  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: penpot
      POSTGRES_USER: penpot
      POSTGRES_PASSWORD: penpot
    volumes: ["pg_data:/var/lib/postgresql/data"]
  redis:
    image: redis:7-alpine
volumes:
  pg_data:`,
	},
}
