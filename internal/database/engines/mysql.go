package engines

import "fmt"

// MySQL implements the Engine interface for MySQL.
type MySQL struct{}

func (m *MySQL) Name() string       { return "mysql" }
func (m *MySQL) Versions() []string { return []string{"8.4", "8.0"} }
func (m *MySQL) DefaultPort() int   { return 3306 }

func (m *MySQL) Image(version string) string {
	return fmt.Sprintf("mysql:%s", version)
}

func (m *MySQL) Env(creds Credentials) []string {
	return []string{
		"MYSQL_DATABASE=" + creds.Database,
		"MYSQL_USER=" + creds.User,
		"MYSQL_PASSWORD=" + creds.Password,
		"MYSQL_ROOT_PASSWORD=" + creds.Password,
	}
}

func (m *MySQL) HealthCmd() []string {
	return []string{"mysqladmin", "ping", "-h", "localhost"}
}

func (m *MySQL) ConnectionString(host string, port int, creds Credentials) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		creds.User, creds.Password, host, port, creds.Database)
}

// MariaDB implements the Engine interface for MariaDB.
type MariaDB struct{}

func (m *MariaDB) Name() string       { return "mariadb" }
func (m *MariaDB) Versions() []string { return []string{"11", "10.11"} }
func (m *MariaDB) DefaultPort() int   { return 3306 }

func (m *MariaDB) Image(version string) string {
	return fmt.Sprintf("mariadb:%s", version)
}

func (m *MariaDB) Env(creds Credentials) []string {
	return []string{
		"MARIADB_DATABASE=" + creds.Database,
		"MARIADB_USER=" + creds.User,
		"MARIADB_PASSWORD=" + creds.Password,
		"MARIADB_ROOT_PASSWORD=" + creds.Password,
	}
}

func (m *MariaDB) HealthCmd() []string {
	return []string{"healthcheck.sh", "--connect", "--innodb_initialized"}
}

func (m *MariaDB) ConnectionString(host string, port int, creds Credentials) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		creds.User, creds.Password, host, port, creds.Database)
}
