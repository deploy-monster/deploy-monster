package build

// Dockerfile templates for each project type.
// Each template is a multi-stage build optimized for production.

var dockerfileTemplates = map[ProjectType]string{
	TypeNodeJS: `FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build 2>/dev/null || true

FROM node:22-alpine
WORKDIR /app
COPY --from=builder /app .
EXPOSE 3000
CMD ["npm", "start"]
`,

	TypeNextJS: `FROM node:22-alpine AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci

FROM node:22-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
EXPOSE 3000
CMD ["node", "server.js"]
`,

	TypeVite: `FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY <<'EOF' /etc/nginx/conf.d/default.conf
server {
    listen 80;
    root /usr/share/nginx/html;
    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF
EXPOSE 80
`,

	TypeNuxt: `FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
COPY --from=builder /app/.output ./
EXPOSE 3000
CMD ["node", "server/index.mjs"]
`,

	TypeGo: `FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/server .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /server
EXPOSE 8080
CMD ["/server"]
`,

	TypeRust: `FROM rust:1-alpine AS builder
RUN apk add --no-cache musl-dev
WORKDIR /app
COPY . .
RUN cargo build --release

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/target/release/app /app
EXPOSE 8080
CMD ["/app"]
`,

	TypePython: `FROM python:3.13-slim AS builder
WORKDIR /app
COPY requirements*.txt ./
RUN pip install --no-cache-dir -r requirements.txt

FROM python:3.13-slim
WORKDIR /app
COPY --from=builder /usr/local/lib/python3.13/site-packages /usr/local/lib/python3.13/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin
COPY . .
EXPOSE 8000
CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
`,

	TypePHP: `FROM composer:2 AS deps
WORKDIR /app
COPY composer.json composer.lock ./
RUN composer install --no-dev --optimize-autoloader

FROM php:8.4-fpm-alpine
RUN apk add --no-cache nginx
COPY --from=deps /app/vendor ./vendor
COPY . .
EXPOSE 80
CMD ["php-fpm"]
`,

	TypeJava: `FROM eclipse-temurin:21-jdk-alpine AS builder
WORKDIR /app
COPY . .
RUN if [ -f "mvnw" ]; then ./mvnw package -DskipTests; \
    elif [ -f "gradlew" ]; then ./gradlew build -x test; \
    fi

FROM eclipse-temurin:21-jre-alpine
COPY --from=builder /app/target/*.jar /app.jar
EXPOSE 8080
CMD ["java", "-jar", "/app.jar"]
`,

	TypeDotNet: `FROM mcr.microsoft.com/dotnet/sdk:9.0-alpine AS builder
WORKDIR /app
COPY . .
RUN dotnet publish -c Release -o /publish

FROM mcr.microsoft.com/dotnet/aspnet:9.0-alpine
WORKDIR /app
COPY --from=builder /publish .
EXPOSE 8080
ENTRYPOINT ["dotnet", "app.dll"]
`,

	TypeRuby: `FROM ruby:3.3-alpine AS builder
WORKDIR /app
COPY Gemfile Gemfile.lock ./
RUN bundle install --without development test

FROM ruby:3.3-alpine
WORKDIR /app
COPY --from=builder /usr/local/bundle /usr/local/bundle
COPY . .
EXPOSE 3000
CMD ["bundle", "exec", "rails", "server", "-b", "0.0.0.0"]
`,

	TypeStatic: `FROM nginx:alpine
COPY . /usr/share/nginx/html
COPY <<'EOF' /etc/nginx/conf.d/default.conf
server {
    listen 80;
    root /usr/share/nginx/html;
    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF
EXPOSE 80
`,
}

// GetDockerfileTemplate returns the Dockerfile template for a project type.
func GetDockerfileTemplate(ptype ProjectType) string {
	return dockerfileTemplates[ptype]
}
