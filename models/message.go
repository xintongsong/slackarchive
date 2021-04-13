package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

func TimestampToTime(ts string) (*time.Time, error) {
	if ts == "" {
		return nil, nil
	}
	parts := strings.SplitN(ts, ".", 2)
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	micro, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}

	t := time.Unix(sec, micro*1000)
	return &t, nil
}

func TimeToTimestamp(t time.Time) string {
	ns := t.UTC().UnixNano()

	return fmt.Sprintf("%d.%06d", ns/int64(time.Second), ns%int64(time.Second)/int64(time.Microsecond))

}

// Msg contains information about a slack message. Most
type Message struct {
	ChannelID string   `sql:",pk"`
	Channel   *Channel `json:",omitempty"`

	// Bot messages don't have a UserID
	UserID          string `sql:",pk,fk"`
	User            *User
	Timestamp       *time.Time `sql:",pk"`
	ThreadTimestamp *time.Time `json:"thread_ts,omitempty" `

	Msg *slack.Msg
}

func (m *Message) Merge(message *slack.Msg) error {
	if message.Channel != "" {
		m.ChannelID = message.Channel
	}

	m.UserID = message.User

	var err error
	m.Timestamp, err = TimestampToTime(message.Timestamp)
	if err != nil {
		return err
	}
	m.ThreadTimestamp, err = TimestampToTime(message.ThreadTimestamp)
	if err != nil {
		return err
	}
	m.Msg = message
	return nil
}
