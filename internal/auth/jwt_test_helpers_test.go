package auth

// MustNewJWTService is a test helper that panics if the secret is too short.
func MustNewJWTService(secret string, previousSecrets ...string) *JWTService {
	svc, err := NewJWTService(secret, previousSecrets...)
	if err != nil {
		panic(err)
	}
	return svc
}
