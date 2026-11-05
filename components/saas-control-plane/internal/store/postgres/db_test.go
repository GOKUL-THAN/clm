package postgres

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/m2p/clm/pkg/config"
)

func TestNewDBConnectsUsingConfiguredCredentials(t *testing.T) {
	testDSN := os.Getenv("CLM_TEST_DB_DSN")
	if testDSN == "" {
		t.Skip("CLM_TEST_DB_DSN is not set")
	}

	connectionURL, err := url.Parse(testDSN)
	if err != nil {
		t.Fatalf("parse test database DSN: %v", err)
	}
	if connectionURL.User == nil {
		t.Fatal("test database DSN has no user")
	}
	password, ok := connectionURL.User.Password()
	if !ok {
		t.Fatal("test database DSN has no password")
	}
	port, err := strconv.Atoi(connectionURL.Port())
	if err != nil {
		t.Fatalf("parse test database port: %v", err)
	}
	databaseName := strings.TrimPrefix(connectionURL.Path, "/")
	if databaseName == "" {
		t.Fatal("test database DSN has no database name")
	}

	db, err := NewDB(&config.DatabaseConfig{
		Host:     connectionURL.Hostname(),
		Port:     port,
		User:     connectionURL.User.Username(),
		Password: password,
		DBName:   databaseName,
		SSLMode:  connectionURL.Query().Get("sslmode"),
	})
	if err != nil {
		t.Fatalf("connect with configured database credentials: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close configured database connection: %v", err)
	}
}
