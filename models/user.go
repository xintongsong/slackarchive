package models

import (
	"context"

	"github.com/go-pg/pg"
	"github.com/slack-go/slack"
)

// UserProfile contains all the information details of a given user
type UserProfile struct {
	FirstName          string `json:"first_name,omitempty"`
	LastName           string `json:"last_name,omitempty"`
	RealName           string `json:"real_name,omitempty"`
	RealNameNormalized string `json:"real_name_normalized,omitempty"`
	Email              string `json:"email,omitempty"`
	Skype              string `json:"skype,omitempty"`
	Phone              string `json:"phone,omitempty"`
	Image24            string `json:"image_24,omitempty"`
	Image32            string `json:"image_32,omitempty"`
	Image48            string `json:"image_48,omitempty"`
	Image72            string `json:"image_72,omitempty"`
	Image192           string `json:"image_192,omitempty"`
	ImageOriginal      string `json:"image_original,omitempty"`
	Title              string `json:"title,omitempty"`
}

// User contains all the information of a user
type User struct {
	ID                string      `json:"id" sql:",notnull"`
	Name              string      `json:"name" sql:",notnull"`
	Team              *Team       `json:"-"`
	TeamID            string      `json:"team,omitempty" sql:",notnull"`
	Deleted           bool        `json:"deleted" sql:",notnull"`
	Color             string      `json:"color,omitempty"`
	Profile           UserProfile `json:"profile"`
	IsBot             bool        `json:"is_bot,omitempty"`
	IsAdmin           bool        `json:"is_admin,omitempty"`
	IsOwner           bool        `json:"is_owner,omitempty"`
	IsPrimaryOwner    bool        `json:"is_primary_owner,omitempty"`
	IsRestricted      bool        `json:"is_restricted,omitempty"`
	IsUltraRestricted bool        `json:"is_ultra_restricted,omitempty"`
	HasFiles          bool        `json:"has_files,omitempty"`
	Presence          string      `json:"presence,omitempty"`
}

func (u *User) AfterSelect(_ context.Context, _ pg.DB) error {
	if u.Team != nil {
		u.TeamID = u.Team.ID
	}
	return nil
}

func (u *User) MergeBot(bot *slack.Bot) {
	u.Name = bot.Name
	u.Deleted = bot.Deleted
	// No, this isn't a typo. The sizes don't match, but this is close
	u.Profile.Image32 = bot.Icons.Image36
	u.Profile.Image48 = bot.Icons.Image48
	u.Profile.Image72 = bot.Icons.Image72
}
