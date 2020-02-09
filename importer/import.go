package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/ashb/slackarchive/config"
	"github.com/ashb/slackarchive/models"
	"github.com/ashb/slackarchive/utils"
	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
	"github.com/nlopes/slack"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("importer")

func New(conf *config.Config, debug bool) *Importer {
	opts, err := pg.ParseURL(conf.Database.DSN)
	if err != nil {
		panic(err)
	}
	db := pg.Connect(opts)
	if debug {
		db.AddQueryHook(models.DBLogger{Logger: log})
	}

	return &Importer{
		db:   db,
		conf: conf,
	}
}

type Importer struct {
	db   orm.DB
	conf *config.Config
}

type TeamImporter struct {
	*Importer

	client *slack.Client
	team   *models.Team
}

func (i *Importer) importTeam(client *slack.Client, token string) (*models.Team, error) {
	team, err := client.GetTeamInfo()
	if err != nil {
		return nil, err
	}

	var t models.Team
	if err := utils.Merge(&t, *team); err != nil {
		return nil, err
	}

	if err := i.db.Model(&t).WherePK().Select(); err == nil {
		log.Debug("Team already exists: ", t.ID)
		_, err = i.db.Model(&t).WherePK().Update(&t)
	} else if err != pg.ErrNoRows {
		return nil, err
	} else {
		_, err = i.db.Model(&t).Insert()
	}

	return &t, err
}

func (i *TeamImporter) importUser(user slack.User) error {
	u := models.User{ID: user.ID, TeamID: i.team.ID}
	if err := i.db.Model(&u).WherePK().Select(); err == nil {
		log.Debugf("User already exists: %s", u.ID)
		return nil
	} else if err != pg.ErrNoRows {
		return err
	}
	if err := utils.Merge(&u, user); err != nil {
		return fmt.Errorf("Error merging user(%s): %s", user.ID, err.Error())
	}

	if _, err := i.db.Model(&u).Insert(); err != nil {
		return fmt.Errorf("Error upserting user(%s): %s", user.ID, err.Error())
	}
	return nil
}

func (i *TeamImporter) importBotUser(bot_id string) error {
	u := &models.User{ID: bot_id, TeamID: i.team.ID}
	if err := i.db.Model(u).WherePK().Select(); err == nil {
		log.Debugf("Bot already exists: %s", u.ID)
		return nil
	} else if err != pg.ErrNoRows {
		return err
	}

	bot, err := i.client.GetBotInfo(bot_id)
	if err != nil {
		return fmt.Errorf("Error querying bot(%s): %s", u.ID, err.Error())
	}

	u.MergeBot(bot)

	if _, err := i.db.Model(u).Insert(); err != nil {
		return fmt.Errorf("Error upserting bot user(%s): %s", u.ID, err.Error())
	}
	return nil
}

func (i *TeamImporter) importUsers(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer f.Close()

	slackbot := models.User{
		TeamID: i.team.ID,
		ID:     "USLACKBOT",
	}
	if _, err := i.db.Model(&slackbot).OnConflict("DO NOTHING").Insert(); err != nil {
		return err
	}

	var users []slack.User
	if err := json.NewDecoder(f).Decode(&users); err != nil {
		return err
	}

	for _, user := range users {
		if err := i.importUser(user); err != nil {
			log.Error(err.Error())
			continue
		}
	}

	return nil
}

func (i *TeamImporter) importChannels(path string) (map[string]models.Channel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	channelsMap := map[string]models.Channel{}

	var channels []slack.Channel
	if err := json.NewDecoder(f).Decode(&channels); err != nil {
		return nil, err
	}

	for _, channel := range channels {
		c := models.Channel{ID: channel.ID, TeamID: i.team.ID}

		if err := i.db.Model(&c).WhereStruct(c).Select(); err == nil {
			channelsMap[c.Name] = c

			log.Debugf("Channel already exists: %s", c.ID)
			// found
			continue
		} else if err != pg.ErrNoRows {
			return nil, err
		}

		if err := utils.Merge(&c, channel); err != nil {
			log.Errorf("Error merging channel(%s): %s", channel.ID, err.Error())
			continue
		}

		if _, err = i.db.Model(&c).Insert(); err != nil {
			log.Errorf("Error inserting channel(%s): %s", channel.ID, err.Error())
			continue
		}

		channelsMap[c.Name] = c
	}

	return channelsMap, nil
}

func (ti *TeamImporter) importMessages(channelID string, path string) error {

	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer f.Close()

	var messages []slack.Message
	if err := json.NewDecoder(f).Decode(&messages); err != nil {
		return err
	}

	for _, message := range messages {

		if message.Type == "message" && message.SubType == "bot_message" {
			if err := ti.importBotUser(message.BotID); err != nil {
				log.Errorf("Error importing bot: %s", err.Error())
				continue
			}
			message.User = message.BotID
		}

		m := &models.Message{ChannelID: channelID}
		if err := m.Merge(message); err != nil {
			log.Errorf("Error merging message: %s", err.Error())
			continue
		}
		if err := ti.db.Model(m).WherePK().Select(); err == nil {
			//	log.Debug("Message already exists: %s", m.ID)
		} else if err != pg.ErrNoRows {
			return err
		} else {
			_, err = ti.db.Model(m).Insert()
			if err != nil {

				panic(err)
			}
		}
	}

	return nil
}

func (i *Importer) Import(token string, importPath string) (*TeamImporter, error) {
	client := slack.New(token)

	team, err := i.importTeam(client, token)
	if err != nil {
		log.Errorf("GetTeamInfo: %s", err.Error())
		return nil, err
	}

	ti := &TeamImporter{
		Importer: i,

		client: client,
		team:   team,
	}

	importPath = path.Join(importPath, team.Domain)

	if err := ti.importUsers(path.Join(importPath, "users.json")); err != nil {
		log.Errorf("importUsers: %s", err.Error())
	}
	var channels map[string]models.Channel
	channels, err = ti.importChannels(path.Join(importPath, "channels.json"))
	if err != nil {
		log.Errorf("importChannel: %s", err.Error())
	}

	for _, channel := range channels {
		channelPath := path.Join(importPath, channel.Name)
		filepath.Walk(channelPath, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}

			log.Infof("Importing path: %s", path)

			if err := ti.importMessages(channel.ID, path); err != nil {
				log.Errorf("importMessages: %s", err.Error())
			}

			return nil
		})
	}

	return ti, nil
}
