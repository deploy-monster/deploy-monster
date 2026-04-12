package build

import (
	"os"
	"path/filepath"
	"testing"
)

func setupDir(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		path := filepath.Join(dir, f)
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte("{}"), 0644)
	}
	return dir
}

func TestDetect_Dockerfile(t *testing.T) {
	dir := setupDir(t, "Dockerfile", "main.go")
	result := Detect(dir)
	if result.Type != TypeDockerfile {
		t.Errorf("expected dockerfile, got %s", result.Type)
	}
}

func TestDetect_NextJS(t *testing.T) {
	dir := setupDir(t, "package.json", "next.config.js")
	result := Detect(dir)
	if result.Type != TypeNextJS {
		t.Errorf("expected nextjs, got %s", result.Type)
	}
}

func TestDetect_Vite(t *testing.T) {
	dir := setupDir(t, "package.json", "vite.config.ts")
	result := Detect(dir)
	if result.Type != TypeVite {
		t.Errorf("expected vite, got %s", result.Type)
	}
}

func TestDetect_Go(t *testing.T) {
	dir := setupDir(t, "go.mod", "main.go")
	result := Detect(dir)
	if result.Type != TypeGo {
		t.Errorf("expected go, got %s", result.Type)
	}
}

func TestDetect_Python(t *testing.T) {
	dir := setupDir(t, "requirements.txt", "app.py")
	result := Detect(dir)
	if result.Type != TypePython {
		t.Errorf("expected python, got %s", result.Type)
	}
}

func TestDetect_Rust(t *testing.T) {
	dir := setupDir(t, "Cargo.toml", "src/main.rs")
	result := Detect(dir)
	if result.Type != TypeRust {
		t.Errorf("expected rust, got %s", result.Type)
	}
}

func TestDetect_Static(t *testing.T) {
	dir := setupDir(t, "index.html", "style.css")
	result := Detect(dir)
	if result.Type != TypeStatic {
		t.Errorf("expected static, got %s", result.Type)
	}
}

func TestDetect_Unknown(t *testing.T) {
	dir := setupDir(t, "random.txt")
	result := Detect(dir)
	if result.Type != TypeUnknown {
		t.Errorf("expected unknown, got %s", result.Type)
	}
}

func TestDetect_DockerCompose(t *testing.T) {
	dir := setupDir(t, "docker-compose.yml")
	result := Detect(dir)
	if result.Type != TypeDockerCompose {
		t.Errorf("expected docker-compose, got %s", result.Type)
	}
}

func TestDetect_Java(t *testing.T) {
	dir := setupDir(t, "pom.xml", "src/main/java/App.java")
	result := Detect(dir)
	if result.Type != TypeJava {
		t.Errorf("expected java, got %s", result.Type)
	}
}

func TestDetect_PHP(t *testing.T) {
	dir := setupDir(t, "composer.json", "index.php")
	result := Detect(dir)
	if result.Type != TypePHP {
		t.Errorf("expected php, got %s", result.Type)
	}
}

func TestDetect_DotNet(t *testing.T) {
	dir := setupDir(t, "MyApp.csproj", "Program.cs")
	result := Detect(dir)
	if result.Type != TypeDotNet {
		t.Errorf("expected dotnet, got %s", result.Type)
	}
}

func TestDockerfileTemplate_Exists(t *testing.T) {
	types := []ProjectType{TypeNodeJS, TypeNextJS, TypeVite, TypeGo, TypePython, TypeRust, TypePHP, TypeJava, TypeDotNet, TypeRuby, TypeStatic, TypeNuxt}
	for _, pt := range types {
		tmpl := GetDockerfileTemplate(pt)
		if tmpl == "" {
			t.Errorf("no template for %s", pt)
		}
	}
}
