package compose

import (
	"testing"
)

const testCompose = `
services:
  web:
    image: nginx:alpine
    ports:
      - "80:80"
    depends_on:
      - api
    environment:
      - NODE_ENV=production
    labels:
      monster.enable: "true"
  api:
    image: node:22-alpine
    environment:
      DATABASE_URL: postgres://db:5432/app
    depends_on:
      - db
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: user
      POSTGRES_PASSWORD: secret
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
`

func TestParse(t *testing.T) {
	cf, err := Parse([]byte(testCompose))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(cf.Services) != 3 {
		t.Errorf("expected 3 services, got %d", len(cf.Services))
	}

	web := cf.Services["web"]
	if web == nil {
		t.Fatal("web service not found")
	}
	if web.Image != "nginx:alpine" {
		t.Errorf("expected nginx:alpine, got %s", web.Image)
	}

	db := cf.Services["db"]
	if db == nil {
		t.Fatal("db service not found")
	}
	if db.ResolvedEnv["POSTGRES_DB"] != "app" {
		t.Errorf("expected POSTGRES_DB=app, got %s", db.ResolvedEnv["POSTGRES_DB"])
	}
}

func TestParse_EmptyServices(t *testing.T) {
	_, err := Parse([]byte(`version: "3"`))
	if err == nil {
		t.Error("expected error for empty services")
	}
}

func TestDependencyOrder(t *testing.T) {
	cf, _ := Parse([]byte(testCompose))
	order := cf.DependencyOrder()

	indexOf := func(name string) int {
		for i, n := range order {
			if n == name {
				return i
			}
		}
		return -1
	}

	// db must come before api, api before web
	if indexOf("db") > indexOf("api") {
		t.Error("db should come before api")
	}
	if indexOf("api") > indexOf("web") {
		t.Error("api should come before web")
	}
}

func TestInterpolate(t *testing.T) {
	input := []byte(`
services:
  app:
    image: ${IMAGE_NAME:-myapp:latest}
    environment:
      DB_HOST: ${DB_HOST}
`)

	vars := map[string]string{
		"DB_HOST": "database.internal",
	}

	result := Interpolate(input, vars)
	s := string(result)

	if !contains(s, "myapp:latest") {
		t.Error("expected default value 'myapp:latest'")
	}
	if !contains(s, "database.internal") {
		t.Error("expected interpolated DB_HOST")
	}
}

func TestResolveEnv_Map(t *testing.T) {
	env := resolveEnv(map[string]any{
		"KEY1": "val1",
		"KEY2": 42,
	})

	if env["KEY1"] != "val1" {
		t.Errorf("expected val1, got %s", env["KEY1"])
	}
}

func TestResolveEnv_List(t *testing.T) {
	env := resolveEnv([]any{
		"KEY1=val1",
		"KEY2=val2",
	})

	if env["KEY1"] != "val1" || env["KEY2"] != "val2" {
		t.Error("list env parsing failed")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
