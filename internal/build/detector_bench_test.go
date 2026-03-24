package build

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkDetect_GoProject(b *testing.B) {
	dir := b.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Detect(dir)
	}
}

func BenchmarkDetect_NextJS(b *testing.B) {
	dir := b.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "next.config.js"), []byte("{}"), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Detect(dir)
	}
}
