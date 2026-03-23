package enterprise

import (
	"encoding/json"
	"sync"
)

// Branding holds white-label customization settings.
// Stored in the database per tenant (or globally for the platform).
type Branding struct {
	LogoURL      string `json:"logo_url"`
	LogoDarkURL  string `json:"logo_dark_url"`
	FaviconURL   string `json:"favicon_url"`
	AppName      string `json:"app_name"`       // Replace "DeployMonster"
	Domain       string `json:"domain"`          // Custom platform domain
	PrimaryColor string `json:"primary_color"`   // Hex color
	AccentColor  string `json:"accent_color"`    // Hex color
	Copyright    string `json:"copyright"`       // Footer text
	SupportEmail string `json:"support_email"`
	SupportURL   string `json:"support_url"`
	HidePoweredBy bool  `json:"hide_powered_by"` // Hide "Powered by DeployMonster"
	CustomCSS    string `json:"custom_css"`       // Injected into UI head
}

// DefaultBranding returns the default DeployMonster branding.
func DefaultBranding() *Branding {
	return &Branding{
		AppName:      "DeployMonster",
		PrimaryColor: "#10b981",
		AccentColor:  "#8b5cf6",
		Copyright:    "DeployMonster by ECOSTACK TECHNOLOGY",
		SupportEmail: "support@deploy.monster",
	}
}

// BrandingStore caches branding per tenant with a fallback to platform defaults.
type BrandingStore struct {
	mu       sync.RWMutex
	platform *Branding
	tenants  map[string]*Branding
}

// NewBrandingStore creates a branding store with default platform branding.
func NewBrandingStore() *BrandingStore {
	return &BrandingStore{
		platform: DefaultBranding(),
		tenants:  make(map[string]*Branding),
	}
}

// GetPlatform returns the platform-level branding.
func (bs *BrandingStore) GetPlatform() *Branding {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.platform
}

// SetPlatform updates platform-level branding.
func (bs *BrandingStore) SetPlatform(b *Branding) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.platform = b
}

// GetTenant returns tenant-specific branding, falling back to platform defaults.
func (bs *BrandingStore) GetTenant(tenantID string) *Branding {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	if b, ok := bs.tenants[tenantID]; ok {
		return b
	}
	return bs.platform
}

// SetTenant updates tenant-specific branding.
func (bs *BrandingStore) SetTenant(tenantID string, b *Branding) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.tenants[tenantID] = b
}

// ToJSON serializes branding for the frontend.
func (b *Branding) ToJSON() string {
	data, _ := json.Marshal(b)
	return string(data)
}
