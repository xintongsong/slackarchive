package models

import (
	"github.com/go-pg/pg"
	logging "github.com/op/go-logging"
)

type DBLogger struct{ *logging.Logger }

func (d DBLogger) BeforeQuery(q *pg.QueryEvent) {}

func (d DBLogger) AfterQuery(q *pg.QueryEvent) {
	d.Debug(q.FormattedQuery())
}
