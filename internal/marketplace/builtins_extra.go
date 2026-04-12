package marketplace

// Additional marketplace templates to reach 20+ total.
func init() {
	builtinTemplates = append(builtinTemplates, extraTemplates...)
}

var extraTemplates = []*Template{
	{
		Slug: "strapi", Name: "Strapi", Category: "cms",
		Description: "Open-source headless CMS built with Node.js",
		Tags:        []string{"cms", "headless", "api"}, Author: "Strapi", Version: "5",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 1024},
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
		Slug: "umami", Name: "Umami", Category: "analytics",
		Description: "Simple, fast, privacy-focused web analytics",
		Tags:        []string{"analytics", "privacy"}, Author: "Umami", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
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
		Slug: "open-webui", Name: "Open WebUI", Category: "ai",
		Description: "Self-hosted ChatGPT-like interface for Ollama and OpenAI",
		Tags:        []string{"ai", "chat", "llm"}, Author: "Open WebUI", Version: "latest",
		Verified: true, Featured: true,
		MinResources: ResourceReq{MemoryMB: 1024, DiskMB: 2048},
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
		Slug: "meilisearch", Name: "Meilisearch", Category: "search",
		Description: "Lightning-fast search engine — open-source alternative to Algolia",
		Tags:        []string{"search", "api"}, Author: "Meilisearch", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
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
		Slug: "nocodb", Name: "NocoDB", Category: "database",
		Description: "Open-source Airtable alternative — turn any database into a spreadsheet",
		Tags:        []string{"database", "spreadsheet", "no-code"}, Author: "NocoDB", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
		ComposeYAML: `services:
  nocodb:
    image: nocodb/nocodb:latest
    ports: ["8080:8080"]
    volumes: ["nocodb_data:/usr/app/data"]
volumes:
  nocodb_data:`,
	},
	{
		Slug: "portainer", Name: "Portainer CE", Category: "devtools",
		Description: "Container management UI for Docker and Kubernetes",
		Tags:        []string{"docker", "containers", "management"}, Author: "Portainer", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 256, DiskMB: 256},
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
		Slug: "grafana", Name: "Grafana", Category: "monitoring",
		Description: "Open-source observability and monitoring dashboards",
		Tags:        []string{"monitoring", "dashboards", "visualization"}, Author: "Grafana Labs", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 512},
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
		Slug: "directus", Name: "Directus", Category: "cms",
		Description: "Open-source headless CMS with real-time API and beautiful admin",
		Tags:        []string{"cms", "headless", "api", "admin"}, Author: "Directus", Version: "latest",
		Verified:     true,
		MinResources: ResourceReq{MemoryMB: 512, DiskMB: 1024},
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
}
