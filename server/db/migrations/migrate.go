package migrations

import (
	"context"
	"database/sql"
	"embed"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// Up applies the embedded PostgreSQL migrations. It is invoked explicitly by
// the API's -migrate command, never during normal application startup.
func Up(ctx context.Context, dsn string) error {
	connection, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer connection.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, connection, files)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
