package marketplace

func init() {
	builtinTemplates = append(builtinTemplates, moreTemplates...)
}

var moreTemplates = []*Template{
	{
		Slug: "immich", Name: "Immich", Category: "storage",
		Description: "Self-hosted photo and video backup — Google Photos alternative",
		Tags: []string{"photos", "backup", "media"}, Author: "Immich", Version: "latest",
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
		Tags: []string{"documents", "ocr", "archive"}, Author: "Paperless-ngx", Version: "latest",
		Verified: true,
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
		Tags: []string{"markdown", "wiki", "collaboration"}, Author: "HedgeDoc", Version: "latest",
		Verified: true,
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
		Tags: []string{"budget", "finance", "money"}, Author: "Actual Budget", Version: "latest",
		Verified: true,
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
		Tags: []string{"bookmarks", "links", "archive"}, Author: "Linkwarden", Version: "latest",
		Verified: true,
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
}
