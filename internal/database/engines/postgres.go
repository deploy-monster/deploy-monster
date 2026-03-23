package engines

import "fmt"

// Postgres implements the Engine interface for PostgreSQL.
type Postgres struct{}

func (p *Postgres) Name() string        { return "postgres" }
func (p *Postgres) Versions() []string  { return []string{"17", "16", "15", "14"} }
func (p *Postgres) DefaultPort() int    { return 5432 }

func (p *Postgres) Image(version string) string {
	return fmt.Sprintf("postgres:%s-alpine", version)
}

func (p *Postgres) Env(creds Credentials) []string {
	return []string{
		"POSTGRES_DB=" + creds.Database,
		"POSTGRES_USER=" + creds.User,
		"POSTGRES_PASSWORD=" + creds.Password,
	}
}

func (p *Postgres) HealthCmd() []string {
	return []string{"pg_isready", "-U", "postgres"}
}

func (p *Postgres) ConnectionString(host string, port int, creds Credentials) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		creds.User, creds.Password, host, port, creds.Database)
}
