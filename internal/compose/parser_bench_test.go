package compose

import "testing"

var benchYAML = []byte(`
services:
  web:
    image: nginx:alpine
    ports: ["80:80"]
    depends_on: [api]
  api:
    image: node:22-alpine
    environment:
      DB_URL: postgres://db:5432/app
    depends_on: [db]
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: app
    volumes: ["data:/var/lib/postgresql/data"]
volumes:
  data:
`)

func BenchmarkParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Parse(benchYAML)
	}
}

func BenchmarkDependencyOrder(b *testing.B) {
	cf, _ := Parse(benchYAML)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cf.DependencyOrder()
	}
}

func BenchmarkInterpolate(b *testing.B) {
	vars := map[string]string{"DB_PASSWORD": "secret123", "APP_PORT": "3000"}
	template := []byte(`
services:
  app:
    environment:
      DB_PASSWORD: ${DB_PASSWORD:-default}
      PORT: ${APP_PORT}
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Interpolate(template, vars)
	}
}
