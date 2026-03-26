package engines

import "fmt"

// Redis implements the Engine interface for Redis.
type Redis struct{}

func (r *Redis) Name() string       { return "redis" }
func (r *Redis) Versions() []string { return []string{"7"} }
func (r *Redis) DefaultPort() int   { return 6379 }

func (r *Redis) Image(version string) string {
	return fmt.Sprintf("redis:%s-alpine", version)
}

func (r *Redis) Env(creds Credentials) []string {
	if creds.Password != "" {
		return []string{"REDIS_PASSWORD=" + creds.Password}
	}
	return nil
}

func (r *Redis) HealthCmd() []string {
	return []string{"redis-cli", "ping"}
}

func (r *Redis) ConnectionString(host string, port int, creds Credentials) string {
	if creds.Password != "" {
		return fmt.Sprintf("redis://:%s@%s:%d", creds.Password, host, port)
	}
	return fmt.Sprintf("redis://%s:%d", host, port)
}

// MongoDB implements the Engine interface for MongoDB.
type MongoDB struct{}

func (m *MongoDB) Name() string       { return "mongodb" }
func (m *MongoDB) Versions() []string { return []string{"7"} }
func (m *MongoDB) DefaultPort() int   { return 27017 }

func (m *MongoDB) Image(version string) string {
	return fmt.Sprintf("mongo:%s", version)
}

func (m *MongoDB) Env(creds Credentials) []string {
	return []string{
		"MONGO_INITDB_ROOT_USERNAME=" + creds.User,
		"MONGO_INITDB_ROOT_PASSWORD=" + creds.Password,
		"MONGO_INITDB_DATABASE=" + creds.Database,
	}
}

func (m *MongoDB) HealthCmd() []string {
	return []string{"mongosh", "--eval", "db.adminCommand('ping')"}
}

func (m *MongoDB) ConnectionString(host string, port int, creds Credentials) string {
	return fmt.Sprintf("mongodb://%s:%s@%s:%d/%s?authSource=admin",
		creds.User, creds.Password, host, port, creds.Database)
}
