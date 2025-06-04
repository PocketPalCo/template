package config

import "testing"

func TestDbConnectionString(t *testing.T) {
	cfg := Config{
		DbUser:         "testuser",
		DbPassword:     "p@ss word",
		DbHost:         "localhost",
		DbPort:         5432,
		DbDatabaseName: "testdb",
		DbSSLMode:      "disable",
	}

	expected := "postgresql://testuser:p%40ss+word@localhost:5432/testdb?sslmode=disable"

	if got := cfg.DbConnectionString(); got != expected {
		t.Errorf("DbConnectionString() = %q, want %q", got, expected)
	}
}
