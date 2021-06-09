package mssql

import (
	"fmt"
	"log"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Auth is for the MSSQL Server connection details
type Auth struct {
	Server   string // Ex: "changeme.database.windows.net" or "localhost"
	Port     string // Default: "1433"
	Instance string // Default: ""
	Username string // Ex: "SA" (or not, we hope)
	Password string // Ex: "secret",
	Catalog  string // Ex: "databasename"
}

// NewConnection returns a new MSSQL connection after some simple checks and
// explicitly setting some defaults
func (auth *Auth) NewConnection() (*sqlx.DB, error) {
	log.Printf("[MS-SQL] [NewConnection] %s\n", auth.String())

	// rather than escape table names, just ignore them
	if len(auth.Catalog) <= 0 || strings.ContainsAny(auth.Catalog, `[]`) {
		return nil, fmt.Errorf("invalid schema identifier name '%s'", auth.Catalog)
	}

	db, err := sqlx.Open("sqlserver", auth.SecretString())
	if err != nil {
		return nil, err
	}

	// Just a batch job, so we don't need much in the way of a connection pool.NewConnection
	db.SetMaxIdleConns(2) // default is 2

	// We don't have concurrent requests, so this doesn't matter, but whatever - best to be explicit
	db.SetMaxOpenConns(0) // default is 0 (unlimited)

	// Connections should only last fractions of a second (we hope) - again, just being explicit
	db.SetConnMaxLifetime(0) // default is 0 (unlimited)

	return db, nil
}

// String outputs a fairly safe version of the connection string
func (auth *Auth) String() string {
	show := 2
	if len(auth.Password) < show {
		auth.Password = "ERR_"
	}
	return fmt.Sprintf(
		"sqlserver://%s:%s@%s:%s/%s?database=%s",
		auth.Username, auth.Password[0:show]+strings.Repeat("*", 12-show), auth.Server,
		auth.Port, auth.Instance, auth.Catalog,
	)
}

// SecretString gives back the full, credentialed access string
func (auth *Auth) SecretString() string {
	return fmt.Sprintf(
		"sqlserver://%s:%s@%s:%s/%s?database=%s",
		auth.Username, auth.Password, auth.Server,
		auth.Port, auth.Instance, auth.Catalog,
	)
}
