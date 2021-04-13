package models

import (
	"net/url"

	"github.com/slack-go/slack"

	"github.com/go-pg/pg/orm"
	"github.com/go-pg/pg/urlvalues"
)

type Channel struct {
	ID        string
	Name      string `sql:",notnull"`
	Team      *Team
	TeamID    string `sql:",notnull"`
	IsChannel bool   `sql:",notnull"`
	//Created    time.Time `bson:"created"`
	CreatorID  string `pg:"fk:User"`
	Creator    *User
	IsArchived bool     `sql:",notnull"`
	IsGeneral  bool     `sql:",notnull"`
	IsGroup    bool     `sql:",notnull"`
	Members    []string `sql:",array"`
	Topic      Topic
	Purpose    Purpose
	IsMember   bool `sql:",notnull"`
	LastRead   string
	//Latest             Message
	UnreadCount        int
	NumMembers         int `sql:",notnull"`
	UnreadCountDisplay int
}

// Purpose contains information about the topic
type Purpose struct {
	Value   string
	Creator string
	LastSet slack.JSONTime
}

// Topic contains information about the topic
type Topic struct {
	Value   string
	Creator string
	LastSet slack.JSONTime
}

type ChannelFilter struct {
	TeamID string
	urlvalues.Pager
}

// NewPager creates a go-pg Pager from net/url.Values using our custom field names
func NewPager(form url.Values) urlvalues.Pager {
	var pager urlvalues.Pager

	var values urlvalues.Values = (urlvalues.Values)(form)

	if val, err := values.Int("offset"); err == nil {
		pager.Offset = val
	}
	if val, err := values.Int("size"); err == nil {
		pager.Limit = val
	}

	return pager
}

func (f *ChannelFilter) Filter(q *orm.Query) (*orm.Query, error) {
	if f.TeamID != "" {
		q = q.Where("?TableAlias.team_id = ?", f.TeamID)
	}

	q = q.Apply(f.Pager.Pagination)

	return q, nil
}
