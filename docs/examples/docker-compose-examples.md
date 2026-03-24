# DeployMonster — Docker Compose Deploy Examples

## WordPress with MySQL

```bash
curl -X POST /api/v1/compose/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "wordpress-blog",
    "yaml": "version: \"3.8\"\nservices:\n  wordpress:\n    image: wordpress:6\n    ports:\n      - \"80:80\"\n    environment:\n      WORDPRESS_DB_HOST: db\n      WORDPRESS_DB_USER: wp\n      WORDPRESS_DB_PASSWORD: ${SECRET:wp_db_pass}\n      WORDPRESS_DB_NAME: wordpress\n    depends_on:\n      - db\n  db:\n    image: mysql:8\n    environment:\n      MYSQL_ROOT_PASSWORD: ${SECRET:mysql_root}\n      MYSQL_DATABASE: wordpress\n      MYSQL_USER: wp\n      MYSQL_PASSWORD: ${SECRET:wp_db_pass}\n    volumes:\n      - db_data:/var/lib/mysql\nvolumes:\n  db_data:"
  }'
```

## n8n Workflow Automation

```bash
curl -X POST /api/v1/compose/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "n8n",
    "yaml": "version: \"3.8\"\nservices:\n  n8n:\n    image: n8nio/n8n:latest\n    ports:\n      - \"5678:5678\"\n    environment:\n      N8N_BASIC_AUTH_ACTIVE: \"true\"\n      N8N_BASIC_AUTH_USER: admin\n      N8N_BASIC_AUTH_PASSWORD: ${SECRET:n8n_pass}\n    volumes:\n      - n8n_data:/home/node/.n8n\nvolumes:\n  n8n_data:"
  }'
```

## Validate Compose YAML

```bash
curl -X POST /api/v1/compose/validate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"yaml": "version: \"3.8\"\nservices:\n  app:\n    image: nginx:alpine\n    ports:\n      - \"80:80\""}'
```
