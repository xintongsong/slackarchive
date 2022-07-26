package bot

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
	logging "github.com/op/go-logging"
	"github.com/pkg/errors"
	"github.com/slack-go/slack"

	"github.com/ashb/slackarchive/config"
	"github.com/ashb/slackarchive/models"
	"github.com/ashb/slackarchive/utils"
)

var log = logging.MustGetLogger("archivebot")

type archiveBot struct {
	session orm.DB

	archivers map[string]*archiveClient
	config    *config.Config
	work      chan func()
}

func New(config *config.Config, db orm.DB) *archiveBot {
	return &archiveBot{
		session:   db,
		config:    config,
		work:      make(chan func(), 100),
		archivers: map[string]*archiveClient{},
	}

}

type archiveClient struct {
	*slack.Client
	ab     *archiveBot
	tokens config.TokenConfig
	Team   *models.Team
	SyncIntervalMinute int
	SyncRecentDay int
}

// Map of last-message we've received per channel.
type lastMessageDates []struct {
	ID         string
	FirstSince *time.Time
}

func (ac *archiveClient) Sync(ctx context.Context, since *time.Time) error {
	db := ac.ab.session

	log.Info("Syncing team (%s)", ac.Team.ID)

	team, err := ac.GetTeamInfo()
	if err != nil {
		return errors.WithMessage(err, "GetTeamInfo")
	}

	// update team
	ac.Team = &models.Team{}
	ac.Team.Token = ac.tokens.BotToken

	if err := utils.Merge(ac.Team, *team); err != nil {
		return errors.Wrapf(err, "could not merging team(%s)", team.ID)
	}

	var exists bool
	if exists, err = db.Model(ac.Team).WherePK().Exists(); exists {
		_, err = db.Model(ac.Team).WherePK().Update(ac.Team)
	} else if err == nil {
		_, err = db.Model(ac.Team).Insert()
	} else {
		log.Error(err.Error())
	}

	if err != nil {
		return errors.Wrapf(err, "could not upsert team(%s)", team.ID)
	}

	log.Info("Syncing team (%s) finished", ac.Team.ID)

	log.Info("Syncing users(%s)", ac.Team.ID)

	pageNum := 0
	count := 0

	page := ac.GetUsersPaginated(slack.GetUsersOptionLimit(1000))
	for err == nil {
		page, err = page.Next(ctx)
		if err == nil {
			pageNum++
			log.Infof("User page %d", pageNum)
			for _, user := range page.Users {
				if err := ac.UpsertUser(user); err != nil {
					log.Errorf("%s", err.Error())
				}

				count++
			}

		} else if rateLimitedError, ok := err.(*slack.RateLimitedError); ok {
			log.Infof("Rate limited for %s", rateLimitedError.RetryAfter)
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(rateLimitedError.RetryAfter):
				err = nil
			}
		}
	}

	log.Info("Syncing users completed(%s): %#v %d", ac.Team.ID, err, count)

	log.Info("Syncing channels (%s)", ac.Team.ID)

	params := slack.GetConversationsParameters{
		ExcludeArchived: false,
		Limit:           100,
	}

	var channels []slack.Channel
	var nextCursor string

	err = nil
	for err == nil {
		channels, nextCursor, err = ac.GetConversationsContext(ctx, &params)

		if err == nil {
			log.Info("Updating info for %d channels", len(channels))
			for _, channel := range channels {
				c := models.Channel{TeamID: ac.Team.ID}

				if err := utils.Merge(&c, channel); err != nil {
					log.Error("Error merging channel(%s): %s", channel.ID, err.Error())
					continue
				}

				_, err := db.Model(&c).OnConflict("(id) DO UPDATE").Insert()

				if err != nil {
					log.Error("Error upserting channel(%s): %s", channel.ID, err.Error())
					continue
				}
			}
			if nextCursor == "" {
				break
			}
			params.Cursor = nextCursor

		} else if rateLimitedError, ok := err.(*slack.RateLimitedError); ok {

			log.Infof("Rate limited for %s", rateLimitedError.RetryAfter)
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(rateLimitedError.RetryAfter):
				err = nil
			}
		}
	}

	if err != nil {
		return errors.Wrapf(err, "could not sync channels(%s)", ac.Team.ID)
	}

	// Now that we have users and channels in the DB we can get a "snapshot" of
	// the latest message date per channel and "backfill" any gaps. Since we
	// have captured the date we can also start the RTM stream and capture new
	// messages without worrying about missing anything
	latest, err := ac.getFirstMessageDatePerChannelSince(since)
	if err != nil {
		return errors.Wrapf(err, "could not get latest message dates per channel (%s)", ac.Team.ID)
	}

	for _, chanInfo := range latest {
		if err := ac.syncChannelMessages(ctx, chanInfo.ID, chanInfo.FirstSince); err != nil {
			log.Error("Error syncing channel messages(%s): %s", chanInfo.ID, err.Error())
		}
	}

	return nil
}

func (ac *archiveClient) getFirstMessageDatePerChannelSince(since *time.Time) (res lastMessageDates, err error) {
	db := ac.ab.session
	query := db.Model((*models.Channel)(nil)).
		Column("id").
		ColumnExpr("min(messages.timestamp) AS first_since").
		Join("LEFT JOIN messages ON channel.id = messages.channel_id")
	if since != nil {
		query = query.Where("messages.timestamp > ?", since)
	}
	query = query.Group("channel.id")
	err = query.Select(&res)

	if err != nil && err != pg.ErrNoRows {
		return res, err
	}
	return res, nil
}

func (ac *archiveClient) syncChannelMessages(ctx context.Context, ChannelID string, Since *time.Time) error {
	log.Info("Syncing latest channel messages: %s (%s)", ChannelID, ac.Team.ID)

	params := &slack.GetConversationHistoryParameters{
		ChannelID: ChannelID,
		Limit:     200,
	}

	if Since != nil {
		params.Oldest = models.TimeToTimestamp(*Since)
		log.Debug("Asking for messages after %s", params.Oldest)
	}

	var history *slack.GetConversationHistoryResponse
	var imported = 0
	var err error

	for err == nil {
		// joining a channel that the bot is already in should not get error
		_, _, _, err = ac.JoinConversation(params.ChannelID)
		if err != nil {
			return errors.Wrap(err, "Error joining channel")
		}

		history, err = ac.GetConversationHistoryContext(ctx, params)

		if err == nil {

			for _, message := range history.Messages {
				if err := ac.NewMessageForChannel(&message.Msg, ChannelID); err != nil {
					panic(err)
				}
				imported++

				if message.Timestamp == message.Timestamp { // is parent of a thread
					importedReplies, e := ac.syncThreadMessages(ctx, ChannelID, message.Timestamp)
					imported += importedReplies

					if e != nil {
						err = nil
						break
					}
				}
			}

			if !history.HasMore {
				break
			}
			params.Oldest = history.Messages[0].Timestamp
			log.Debug("Asking for messages after %s", params.Oldest)

		} else if rateLimitedError, ok := err.(*slack.RateLimitedError); ok {

			log.Infof("Rate limited for %s", rateLimitedError.RetryAfter)
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(rateLimitedError.RetryAfter):
				err = nil
			}
		}
	}
	if err != nil {
		return errors.Wrap(err, "Error retrieving channel history")
	}

	log.Info("Syncing latest channel messages completed: %s - %d new messages", ChannelID, imported)
	return nil
}

func (ac *archiveClient) syncThreadMessages(ctx context.Context, ChannelID string, parentTimeStamp string) (int, error) {
	log.Debug("Syncing thread messages: %s", parentTimeStamp)
	params := &slack.GetConversationRepliesParameters{
		ChannelID: ChannelID,
		Timestamp: parentTimeStamp,
	}

	var imported = 0
	var err error

	for err == nil {
		replies, hasMore, nextCursor, e := ac.GetConversationRepliesContext(ctx, params)
		if e == nil {
			for _, reply := range replies {
				if err := ac.NewMessageForChannel(&reply.Msg, ChannelID); err != nil {
					panic(err)
				}
			}
			imported += len(replies)

			params.Cursor = nextCursor
			if !hasMore {
				break
			}
		} else if rateLimitedError, ok := e.(*slack.RateLimitedError); ok {
			log.Infof("Rate limited for %s", rateLimitedError.RetryAfter)
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(rateLimitedError.RetryAfter):
				err = nil
			}
		}
	}

	if err == nil {
		log.Debug("Syncing thread messages completed: %s - %d messages", parentTimeStamp, imported)
	}

	return imported, err
}

/* ImportBotUser will create a record for the given botID if it doesn't already
*  exist in the database.
*
*  It will ask the Slack API for info about this bot user
 */
func (ac *archiveClient) ImportBotUser(botID string) error {
	u := &models.User{ID: botID, TeamID: ac.Team.ID}
	if err := ac.ab.session.Model(u).WherePK().Select(); err == nil {
		return nil
	} else if err != pg.ErrNoRows {
		return err
	}

	bot, err := ac.GetBotInfo(botID)
	if err != nil {
		return fmt.Errorf("Error querying bot(%s): %s", u.ID, err.Error())
	}
	return ac.UpsertBotUser(*bot)

}

func (ac *archiveClient) UpsertUser(user slack.User) error {
	u := &models.User{}
	if err := utils.Merge(u, user); err != nil {
		return errors.Wrapf(err, "error merging user(%s)", user.ID)
	}

	u.TeamID = ac.Team.ID
	_, err := ac.ab.session.Model(u).OnConflict("(id) DO UPDATE").Insert()
	return errors.Wrapf(err, "error upserting user (%s)", user.ID)
}

func (ac *archiveClient) UpsertBotUser(bot slack.Bot) error {
	u := &models.User{ID: bot.ID, TeamID: ac.Team.ID}
	u.MergeBot(&bot)

	_, err := ac.ab.session.Model(u).OnConflict("(id) DO UPDATE").Set("name = excluded.NAME, deleted = excluded.deleted, profile = excluded.profile").Insert()

	return errors.Wrapf(err, "error upserting bot user(%s)", u.ID)
}

func (ac *archiveClient) NewMessage(msg *slack.Msg) error {
	return ac.NewMessageForChannel(msg, "")
}

func (ac *archiveClient) NewMessageForChannel(msg *slack.Msg, channelID string) error {
	m := &models.Message{ChannelID: channelID}
	if err := m.Merge(msg); err != nil {

		return errors.WithStack(err)
	}

	if msg.Type == "message" && msg.SubType == "bot_message" {
		if err := ac.ImportBotUser(msg.BotID); err != nil {
			return errors.Wrap(err, "error importing bot")
		}
		m.UserID = msg.BotID
	}

	_, err := ac.ab.session.Model(m).OnConflict(`(channel_id, user_id, "timestamp") DO UPDATE`).Insert()
	return errors.Wrap(err, "error upserting message")
}

func (ac *archiveClient) Bot() {

	ctx := context.Background()

	rtm := slack.New(
		ac.tokens.BotToken,
		slack.OptionDebug(ac.Client.Debug()),
	).NewRTM()
	go rtm.ManageConnection()

Loop:
	for {
		select {
		case msg := <-rtm.IncomingEvents:
			switch ev := msg.Data.(type) {
			case *slack.HelloEvent:
				// Ignore hello
			case *slack.ConnectedEvent:
				log.Debug("Connection counter: %d %s", ev.ConnectionCount, ev.Info.Team.Domain)

				// Re-sync as we might have missed messages in the mean time
				if ev.ConnectionCount > 0 {
					go ac.Sync(ctx, nil)
				}
			case *slack.MessageEvent:
				msg := slack.Message(*msg.Data.(*slack.MessageEvent))

				var err error
				switch msg.SubType {
				case "message_replied":
					// Slack "resends" us the original message but with updated thread
					// counts.
					err = ac.NewMessageForChannel(msg.SubMessage, msg.Channel)
				case "bot_message":
					fallthrough
				case "":
					err = ac.NewMessage(&msg.Msg)
				case "message_changed":
					err = ac.NewMessageForChannel(msg.SubMessage, msg.Channel)
				case "message_deleted":
					m := &models.Message{}
					m.Merge(&msg.Msg)
					ac.ab.session.Model(&m).Delete()
				case "channel_join":
					// Ignore this subtype
				default:
					log.Debug("Unknwon message subtype %s", msg.SubType)
					continue
				}

				if err != nil {
					log.Error("Message upsert error: %s", err.Error())
					continue
				}
			case *slack.LatencyReport:
			//	log.Debug("Current latency: %v", ev.Value)
			case *slack.UserTypingEvent:
				log.Debug("User(%s) typing...", msg.Data.(*slack.UserTypingEvent).User)
			case *slack.RTMError:
				log.Debug("Error: %s", ev.Error())
			case *slack.ConnectingEvent:
				log.Debug("Connecting to Real-Time Messages API...")
			case *slack.RTMEvent:
				// 2016/09/04 12:19:18 Unexpected: slack.RTMEvent{Type:"connection_error", Data:(*slack.ConnectionErrorEvent)(0xc4243f6380
				if err, ok := ev.Data.(error); ok {
					log.Error("Event: %s %s", ev.Type, err.Error())
				} else {
					log.Debug("Event: %s", ev.Type)
				}
			case *slack.ReconnectUrlEvent:
				// log.Debug("ReconnectURL: %#v", ev)
			case *slack.InvalidAuthEvent:
				log.Debug("Invalid credentials")
				break Loop
			// case *slack.DesktopNotification:
			case *slack.TeamJoinEvent:
				// TODO: Add new user
				user := msg.Data.(*slack.TeamJoinEvent).User

				u := &models.User{}
				if err := utils.Merge(u, user); err != nil {
					log.Error("Error merging user(%s): %s", user.ID, err.Error())
					continue
				}

				u.Team = ac.Team

				if _, err := ac.ab.session.Model(u).Insert(); err != nil {
					log.Error("Error inserting user(%s): %s", user.ID, err.Error())
					continue
				}
			case *slack.UserChangeEvent:
				user := msg.Data.(*slack.UserChangeEvent).User
				u := &models.User{}
				if err := utils.Merge(u, user); err != nil {
					log.Error("Error merging user(%s): %s", user.ID, err.Error())
					continue
				}

				u.Team = ac.Team

				if _, err := ac.ab.session.Model(u).WherePK().Update(); err != nil {
					log.Error("Error updating user(%s): %s", user.ID, err.Error())
					continue
				}
			case *slack.BotAddedEvent:
				evt := msg.Data.(*slack.BotAddedEvent)
				if err := ac.UpsertBotUser(evt.Bot); err != nil {
					log.Errorf("error upserting bot user(%s): %s", evt.Bot.ID, err)
					continue
				}
			case *slack.BotChangedEvent:
				evt := msg.Data.(*slack.BotChangedEvent)
				if err := ac.UpsertBotUser(evt.Bot); err != nil {
					log.Errorf("error upserting bot user(%s): %s", evt.Bot.ID, err)
					continue
				}

			case *slack.ChannelJoinedEvent:
				// We've been invited to join a new Channel
				evt := msg.Data.(*slack.ChannelJoinedEvent)
				c := models.Channel{TeamID: ac.Team.ID}

				if err := utils.Merge(&c, evt.Channel); err != nil {
					log.Error("Error merging channel(%s): %s", evt.Channel.ID, err.Error())
					continue
				}

				_, err := ac.ab.session.Model(&c).OnConflict("(id) DO UPDATE").Insert()

				if err != nil {
					log.Error("Error upserting channel(%s): %s", evt.Channel.ID, err.Error())
					continue
				}
			case *slack.ChannelRenameEvent:
				evt := msg.Data.(*slack.ChannelRenameInfo)
				ac.ab.session.Model(&models.Channel{
					ID:     evt.ID,
					TeamID: ac.Team.ID,
					Name:   evt.Name,
				}).WherePK().Update()

			case *slack.ReactionAddedEvent:
				// TODO: Work out if we want to store Reactions at all
				log.Debug("Unexpected: %s, %s, %#v", ac.Team.ID, ac.Team.Domain, msg.Data.(*slack.ReactionAddedEvent))

				/*
					-- Check if the reaction type exists:
					select msg->'reactions' @> '[{"name": "clap"}]'::jsonb FROM messages WHERE channel_id = 'CCQ18L37F' and "timestamp" = to_timestamp(1548207132.053000);

					-- Add an (empty) reaction list to a message if it's not there already:
					UPDATE messages
					SET msg = jsonb_set(msg, '{reactions}', msg->'reactions' || '[{"name": "whee", "count": 0}]'::jsonb)
					WHERE NOT (msg->'reactions' @> '[{"name": "whee"}]'::jsonb)
								AND channel_id = 'CCQ18L37F'
								AND "timestamp" = to_timestamp(1548207132.053000);
				*/
			case *slack.ReactionRemovedEvent:
				log.Debug("Unexpected: %s, %s, %#v", ac.Team.ID, ac.Team.Domain, msg.Data.(*slack.ReactionRemovedEvent))
			case *slack.FilePublicEvent:
				log.Debug("Unexpected: %s, %s, %#v", ac.Team.ID, ac.Team.Domain, msg.Data.(*slack.FilePublicEvent))
			case *slack.FileSharedEvent:
				log.Debug("Unexpected: %s, %s, %#v", ac.Team.ID, ac.Team.Domain, msg.Data.(*slack.FileSharedEvent))

			// Events to ignore as we don't care about them
			case *slack.MemberJoinedChannelEvent:
			case *slack.MemberLeftChannelEvent:

			default:
				// Ignore other events..
				log.Debug("Unexpected: %s, %s, %#v", ac.Team.ID, ac.Team.Domain, msg)
			}
		}
	}
}

func (ab *archiveBot) NewArchiveClient(token config.TokenConfig, config config.Config) (*archiveClient, error) {
	ac := archiveClient{
		slack.New(
			token.OAuthToken,
			slack.OptionDebug(false),
		),
		ab,
		token,
		nil,
		config.SyncIntervalMinute,
		config.SyncRecentDay,
	}

	var team *slack.TeamInfo
	var err error
	if team, err = ac.GetTeamInfo(); err != nil {
		return nil, errors.Wrap(err, "error getting team info")
	}
	ac.Team = &models.Team{}
	ac.Team.Token = ac.tokens.BotToken

	if err := utils.Merge(ac.Team, *team); err != nil {
		return nil, errors.Wrapf(err, "error merging team(%s)", team.ID)
	}

	ab.archivers[team.ID] = &ac

	return &ac, nil
}

func (ac *archiveClient) Start() {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				trace := make([]byte, 1024)
				count := runtime.Stack(trace, true)
				log.Error("Error: %s", err)
				log.Debug("Stack of %d bytes: %s", count, trace)
				return
			}
		}()

		syncFunc := func(){
			since := time.Now().Add(time.Hour * time.Duration(-24 * ac.SyncRecentDay))
			if err := ac.Sync(context.Background(), &since); err != nil {
				log.Error("Sync error: %s", err.Error())
				panic(err)
			}
		}

		syncFunc()
		ticker := time.NewTicker(time.Minute * time.Duration(ac.SyncIntervalMinute))
		for range ticker.C {
			syncFunc()
		}
	}()
}

func (ac *archiveClient) RetrieveAll() {
	defer func() {
		if err := recover(); err != nil {
			trace := make([]byte, 1024)
			count := runtime.Stack(trace, true)
			log.Error("Error: %s", err)
			log.Debug("Stack of %d bytes: %s", count, trace)
			return
		}
	}()

	if err := ac.Sync(context.Background(), nil); err != nil {
		log.Error("Sync error: %s", err.Error())
		panic(err)
	}

	log.Info("Init finished. Press 'Ctrl + C' to terminate.")
}

func (ab *archiveBot) worker() {
	// this runs periodically with a random time, specific functions
	// to prevent rate limiting to happen

	for {
		fn := <-ab.work

		// Wait 0 to 2 seconds between operations to avoid hammering rate limits
		wait := rand.Float32() * 2

		log.Info("Waiting for %.02f seconds", wait)

		time.Sleep(time.Duration(wait) * time.Second)

		go fn()
	}
}

func (ab *archiveBot) Reload() {
}

func (ab *archiveBot) Start() {
	go ab.worker()

	for _, token := range ab.config.BotTokens {
		/*var team models.Team
		if err := db.Teams.Find(bson.M{
			"token": token.BotToken,
		}).One(&team); err == nil {
			log.Info("Starting archive bot for team: %s", team.Domain)
		} else {
			log.Info("Starting archive bot for token: %s", token.BotToken)
		}*/
		log.Info("Starting archive bot for token: %s", token.BotToken)

		ac, err := ab.NewArchiveClient(token, *ab.config)
		if err == nil {
		} else if err.Error() == "invalid_auth" || err.Error() == "account_inactive" {
			continue
		} else if err != nil {
			log.Error("Error starting client %s: %s", token, err)
			continue
		}

		ac.Start()
	}
}

func (ab *archiveBot) RetrieveAll() {
	go ab.worker()

	for _, token := range ab.config.BotTokens {
		log.Info("Starting archive bot for token: %s", token.BotToken)

		ac, err := ab.NewArchiveClient(token, *ab.config)
		if err == nil {
		} else if err.Error() == "invalid_auth" || err.Error() == "account_inactive" {
			continue
		} else if err != nil {
			log.Error("Error starting client %s: %s", token, err)
			continue
		}

		ac.RetrieveAll()
	}
}
