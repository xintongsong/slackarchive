package models

import (
	"github.com/go-pg/pg"
	logging "github.com/op/go-logging"
	"github.com/pkg/errors"
)

func Connect(dsn string, debug bool) (*pg.DB, error) {
	opts, err := pg.ParseURL(dsn)
	if err != nil {
		return nil, errors.WithMessage(err, "couldn't create db")
	}
	db := pg.Connect(opts)
	if debug {
		var log = logging.MustGetLogger("models")
		db.AddQueryHook(DBLogger{Logger: log})
	}
	return db, nil
}
