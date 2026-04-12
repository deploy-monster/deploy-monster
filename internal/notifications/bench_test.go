package notifications

import (
	"testing"
)

func BenchmarkDispatcher_RegisterAndGet(b *testing.B) {
	d := NewDispatcher()
	d.RegisterProvider(NewSlackProvider("https://hooks.slack.com/test"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.GetProvider("slack")
	}
}

func BenchmarkDispatcher_RegisterProvider(b *testing.B) {
	provider := NewSlackProvider("https://hooks.slack.com/test")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewDispatcher()
		d.RegisterProvider(provider)
	}
}

func BenchmarkDispatcher_Providers(b *testing.B) {
	d := NewDispatcher()
	d.RegisterProvider(NewSlackProvider("https://hooks.slack.com/test"))
	d.RegisterProvider(NewDiscordProvider("https://discord.com/api/webhooks/123/abc"))
	d.RegisterProvider(NewTelegramProvider("123456:ABC", "chat123"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Providers()
	}
}

func BenchmarkNewSlackProvider(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewSlackProvider("https://hooks.slack.com/services/T00/B00/xxx")
	}
}

func BenchmarkNewDiscordProvider(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewDiscordProvider("https://discord.com/api/webhooks/123/abc")
	}
}
