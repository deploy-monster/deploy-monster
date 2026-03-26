package marketplace

// Extended marketplace templates to reach 50+ total.
func init() {
	builtinTemplates = append(builtinTemplates, extendedTemplates...)
}

var extendedTemplates = []*Template{
	// === Databases ===
	{
		Slug: "postgresql", Name: "PostgreSQL", Category: "database",
		Description: "The world's most advanced open source relational database",
		Tags: []string{"database", "sql", "relational"}, Author: "PostgreSQL", Version: "17",
		Verified: true,
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
		Tags: []string{"database", "sql", "relational"}, Author: "Oracle", Version: "8.4",
		Verified: true,
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
		Tags: []string{"cache", "database", "nosql"}, Author: "Redis Ltd", Version: "7",
		Verified: true,
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
		Tags: []string{"database", "nosql", "document"}, Author: "MongoDB Inc", Version: "8",
		Verified: true,
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
		Tags: []string{"search", "analytics", "logging"}, Author: "Elastic", Version: "8",
		Verified: true,
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
		Tags: []string{"git", "ci/cd", "devops"}, Author: "GitLab", Version: "latest",
		Verified: true,
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
		Tags: []string{"ci", "cd", "containers"}, Author: "Harness", Version: "2",
		Verified: true,
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
		Tags: []string{"ci", "cd", "automation"}, Author: "Woodpecker", Version: "latest",
		Verified: true,
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
		Tags: []string{"media", "streaming", "movies"}, Author: "Jellyfin", Version: "latest",
		Verified: true,
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
		Tags: []string{"music", "streaming", "audio"}, Author: "Navidrome", Version: "latest",
		Verified: true,
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
		Tags: []string{"audiobooks", "podcasts", "media"}, Author: "advplyr", Version: "latest",
		Verified: true,
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
		Tags: []string{"chat", "team", "messaging"}, Author: "Mattermost", Version: "latest",
		Verified: true,
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
		Tags: []string{"chat", "team", "messaging"}, Author: "Rocket.Chat", Version: "latest",
		Verified: true,
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
		Tags: []string{"matrix", "chat", "federation"}, Author: "Matrix.org", Version: "latest",
		Verified: true,
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
		Tags: []string{"office", "documents", "collaboration"}, Author: "ONLYOFFICE", Version: "latest",
		Verified: true,
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
		Tags: []string{"office", "documents", "collaboration"}, Author: "Collabora", Version: "latest",
		Verified: true,
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
		Tags: []string{"wiki", "knowledge", "documentation"}, Author: "Outline", Version: "latest",
		Verified: true,
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
		Tags: []string{"monitoring", "metrics", "alerting"}, Author: "Prometheus", Version: "latest",
		Verified: true,
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
		Tags: []string{"logging", "observability", "grafana"}, Author: "Grafana Labs", Version: "latest",
		Verified: true,
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
		Tags: []string{"tracing", "observability", "grafana"}, Author: "Grafana Labs", Version: "latest",
		Verified: true,
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
		Tags: []string{"proxy", "load-balancer", "ssl"}, Author: "Traefik Labs", Version: "3",
		Verified: true,
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
		Tags: []string{"web-server", "ssl", "proxy"}, Author: "Caddy", Version: "2",
		Verified: true,
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
		Tags: []string{"sso", "oauth", "authentication"}, Author: "goauthentik", Version: "latest",
		Verified: true,
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
		Tags: []string{"ai", "speech", "transcription"}, Author: "OpenAI", Version: "latest",
		Verified: true,
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
		Tags: []string{"ai", "image", "generation"}, Author: "AUTOMATIC1111", Version: "latest",
		Verified: true,
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
		Tags: []string{"ai", "langchain", "rag"}, Author: "DataStax", Version: "latest",
		Verified: true,
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
		Tags: []string{"wiki", "documentation", "knowledge"}, Author: "DokuWiki", Version: "latest",
		Verified: true,
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
		Tags: []string{"wiki", "documentation", "knowledge"}, Author: "BookStack", Version: "latest",
		Verified: true,
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
		Tags: []string{"backend", "baas", "firebase-alternative"}, Author: "Appwrite", Version: "latest",
		Verified: true,
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
		Tags: []string{"backend", "baas", "sqlite"}, Author: "PocketBase", Version: "latest",
		Verified: true,
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
		Tags: []string{"backend", "baas", "postgresql"}, Author: "Supabase", Version: "latest",
		Verified: true,
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
}
