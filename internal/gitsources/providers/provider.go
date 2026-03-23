package providers

import "github.com/deploy-monster/deploy-monster/internal/core"

// Factory creates a GitProvider from an auth token.
type Factory func(token string) core.GitProvider

// Registry holds all available Git provider factories.
var Registry = map[string]Factory{
	"github": NewGitHub,
	"gitlab": NewGitLab,
	"gitea":  NewGitea,
}
