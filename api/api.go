package api

import (
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"sync"
	"time"

	"context"

	errwrap "github.com/pkg/errors"
	autocert "golang.org/x/crypto/acme/autocert"

	config "github.com/ashb/slackarchive/config"
	models "github.com/ashb/slackarchive/models"
	utils "github.com/ashb/slackarchive/utils"
	"github.com/go-pg/pg"
	"github.com/slack-go/slack"

	handlers "github.com/ashb/slackarchive/api/handlers"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"

	"net"
	"strings"

	logging "github.com/op/go-logging"
)

var log = logging.MustGetLogger("slackarchive-api")

type api struct {
	session *pg.DB
	config  *config.Config
	store   *sessions.CookieStore

	wg sync.WaitGroup

	indexChan chan (Message)

	// Registered connections.
	connections map[*connection]bool

	// Register requests from the connections.
	register chan *connection

	// Unregister requests from connections.
	unregister chan *connection
}

func New(config *config.Config) *api {

	log.Info("Starting")

	opts, err := pg.ParseURL(config.Database.DSN)
	if err != nil {
		panic(err)
	}
	db := pg.Connect(opts)
	db.AddQueryHook(models.DBLogger{Logger: log})

	var store = sessions.NewCookieStore(
		[]byte(config.Cookies.AuthenticationKey),
		[]byte(config.Cookies.EncryptionKey),
	)

	return &api{
		session:     db,
		config:      config,
		store:       store,
		indexChan:   make(chan Message),
		connections: map[*connection]bool{},
		register:    make(chan *connection),
		unregister:  make(chan *connection),
	}
}

func (api *api) run() {
	for {
		select {
		case c := <-api.register:
			api.connections[c] = true
		case c := <-api.unregister:
			if _, ok := api.connections[c]; ok {
				delete(api.connections, c)
				close(c.send)
			}
		}
	}
}

func (api *api) teamHandler(ctx *Context) error {
	type TeamResponse struct {
		ID         string `json:"team_id"`
		Domain     string `json:"domain"`
		Name       string `json:"name"`
		IsDisabled bool   `json:"is_disabled"`
		IsHidden   bool   `json:"is_hidden"`

		Plan string                 `json:"plan"`
		Icon map[string]interface{} `json:"icon"`
	}

	response := struct {
		Teams  []TeamResponse `json:"team"`
		Status string         `json:"status"`
	}{}

	var teams []models.Team
	if t, err := api.Team(ctx); err == nil {
		teams = append(teams, *t)
		log.Debugf("api.Team added %+v", t)
	} else {
		qry := ctx.db.Model(&teams).Where("is_disabled = false")
		err := qry.Select()

		if err != nil {
			// TODO: 400/500
			fmt.Printf("Error: %s\n", err.Error())
			return err
		}
	}
	for _, team := range teams {
		tr := TeamResponse{}
		if err := utils.Merge(&tr, team); err != nil {
			return err
		}

		response.Teams = append(response.Teams, tr)
	}

	return ctx.Write(response)
}

type domainFn func(*models.Team, *Context) error

// TODO: only for auth users send more info
type UserResponse struct {
	ID      string `json:"user_id"`
	Name    string `json:"name"`
	Team    string `json:"team"`
	Deleted bool   `json:"deleted"`
	Color   string `json:"color"`
	Profile struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		RealName  string `json:"real_name"`
		// RealNameNormalized string `json:"real_name_normalized"`
		// Email              string `json:"email"`
		// Skype         string `json:"skype"`
		// Phone         string `json:"phone"`
		Image24       string `json:"image_24"`
		Image32       string `json:"image_32"`
		Image48       string `json:"image_48"`
		Image72       string `json:"image_72"`
		Image192      string `json:"image_192"`
		ImageOriginal string `json:"image_original"`
		Title         string `json:"title"`
	} `json:"profile"`
	// IsBot             bool   `json:"is_bot"`
	// IsAdmin           bool   `json:"is_admin"`
	// IsOwner           bool   `json:"is_owner"`
	// IsPrimaryOwner    bool   `json:"is_primary_owner"`
	// IsRestricted      bool   `json:"is_restricted"`
	// IsUltraRestricted bool   `json:"is_ultra_restricted"`
	// HasFiles          bool   `json:"has_files"`
	// Presence          string `json:"presence"`
}

func (api *api) usersHandler(ctx *Context) error {
	response := struct {
		Users      []slack.User `json:"users"`
		TotalCount int64        `json:"total"`
	}{}

	panic("usersHandler not ported")

	//ctx.w.Header().Set("Content-Type", "application/json")
	return ctx.Write(response)
}

/*
	var team *models.Team
	if t, err := api.Team(ctx); err == nil {
		team = t
	} else {
		return err
	}

	team_id := team.ID

	offset := int(0)
	if val, err := strconv.Atoi(ctx.r.FormValue("offset")); err == nil {
		offset = val
	}

	size := int(1000)
	if val, err := strconv.Atoi(ctx.r.FormValue("size")); err == nil {
		size = val
	}

	qry := ctx.db.Users.Find(bson.M{"team": team_id})

	if count, err := qry.Count(); err != nil {
		response.TotalCount = int64(count)
	}

	iter := qry.Skip(offset).Limit(size).Iter()
	defer iter.Close()

	user := models.User{}
	for iter.Next(&user) {
		usr := UserResponse{}
		if err := utils.Merge(&usr, user); err != nil {
			log.Error(err.Error())
		}

		response.Users = append(response.Users, usr)
	}
*/

func (api *api) channelsHandler(ctx *Context) error {
	type ChannelResponse struct {
		ID         string `json:"channel_id"`
		Name       string `json:"name"`
		Team       string `json:"team"`
		IsChannel  bool   `json:"is_channel"`
		IsArchived bool   `json:"is_archived"`
		IsGeneral  bool   `json:"is_general"`
		IsGroup    bool   `json:"is_group"`
		IsStarred  bool   `json:"is_starred"`
		IsMember   bool   `json:"is_member"`
		Purpose    struct {
			Value string `json:"value"`
		} `json:"purpose"`
		NumMembers int `json:"num_members"`
	}

	response := struct {
		Channels   []ChannelResponse `json:"channels"`
		TotalCount int               `json:"total"`
	}{}

	var team *models.Team
	var err error
	if team, err = api.Team(ctx); err != nil {
		return err
	}

	var channels []models.Channel

	filter := &models.ChannelFilter{team.ID, models.NewPager(ctx.r.Form)}
	count, err := ctx.db.Model(&channels).Apply(filter.Filter).SelectAndCount()

	if err != nil {
		return err
	}
	response.TotalCount = count

	for _, channel := range channels {
		chnl := ChannelResponse{}
		if err := utils.Merge(&chnl, channel); err != nil {
			log.Error(err.Error())
		}

		response.Channels = append(response.Channels, chnl)
	}

	//ctx.w.Header().Set("Content-Type", "application/json")
	return ctx.Write(response)
}

func Host(r *http.Request) (string, error) {
	if h := r.FormValue("host"); h != "" {
		return h, nil
	}

	referer := r.Referer()

	if v := r.Header.Get("X-Alt-Referer"); v != "" {
		referer = v
	}

	if v := r.Header.Get("x-alt-referer"); v != "" {
		referer = v
	}

	if u, err := url.Parse(referer); err != nil {
		return "", err
	} else if h, _, err := net.SplitHostPort(u.Host); err == nil {
		return h, nil
	} else {
		return u.Host, nil
	}

}

func (api *api) Team(ctx *Context) (*models.Team, error) {
	host := ""

	r := ctx.r

	if h, err := Host(r); err == nil {
		host = h
	} else {
		return nil, err
	}

	team := &models.Team{
		IsDisabled: false,
		Domain:     host,
	}

	if err := ctx.db.Model(team).WhereStruct(team).Select(); err == nil {
		return team, nil
	}

	team.Domain = api.config.Team
	if err := ctx.db.Model(team).WhereStruct(team).Select(); err == nil {
		return team, nil
	} else {
		log.Errorf("Error: %#v\n", err.Error())
		return nil, fmt.Errorf("Team is disabled or does not exist")
	}
}

func (api *api) messagesHandler(ctx *Context) error {

	response := struct {
		Messages   []slack.Message `json:"messages"`
		TotalCount int             `json:"total"`
		Aggs       struct {
			Buckets map[string]int64 `json:"buckets"`
		} `json:"aggs"`
		Related struct {
			Users map[string]models.User `json:"users"`
		} `json:"related"`
	}{
		Messages: []slack.Message{},
		Aggs: struct {
			Buckets map[string]int64 `json:"buckets"`
		}{
			Buckets: map[string]int64{},
		},
		Related: struct {
			Users map[string]models.User `json:"users"`
		}{
			Users: map[string]models.User{},
		},
	}

	var team *models.Team
	var err error
	if team, err = api.Team(ctx); err != nil {
		return err
	}
	_ = team

	var messages []models.Message
	qry := api.session.Model(&messages)

	var searchQuery string
	if searchQuery = ctx.r.FormValue("q"); searchQuery != "" {
		qry.Where(`?TableAlias.tsv @@ websearch_to_tsquery(?)`, searchQuery)
	}

	qry.Column("Channel._").Where("Channel.team_id = ?", team.ID)

	// check if bot have been removed from the channel
	if channel := ctx.r.FormValue("channel"); channel != "" {
		// TODO: Check our Archive bot is still a member of this channel
		qry.WhereStruct(&models.Message{
			ChannelID: channel,
		})

	} else {
		// TODO: Only the channels where our Archive bot is still a member of this channel
		//panic("Not implemented - no channel")
	}

	if ctx.r.FormValue("qfrom") != "" || ctx.r.FormValue("qto") != "" {
		panic("Not implemented - qfrom/qto")
	}

	if val := ctx.r.FormValue("thread"); val != "" {
		panic("Not implemented - thread")
	}

	var from, to *time.Time
	if from, err = models.TimestampToTime(ctx.r.FormValue("from")); err != nil {
		return errwrap.Wrap(err, "Invalid from value")
	}

	if to, err = models.TimestampToTime(ctx.r.FormValue("to")); err != nil {
		return errwrap.Wrap(err, "Invalid to value")
	}

	if from != nil && to != nil {
		qry.Where("?TableAlias.timestamp BETWEEN ? AND ?", from, to)
	} else if from != nil {
		qry.Where("?TableAlias.timestamp >= ?", from)
	} else if to != nil {
		qry.Where("?TableAlias.timestamp <= ?", to)
	}

	qry.Where(`NOT ?TableAlias."msg" @> '{"hidden": true}'`)
	qry.Where(`?TableAlias."msg"->>'subtype' IS NULL OR ?TableAlias."msg"->>'subtype' NOT IN ('message_changed', 'message_deleted', 'channel_join', 'channel_leave', 'pinned_item')`)

	if val := ctx.r.FormValue("aggs"); val == "1" {

		var res []struct {
			ChannelID string
			Matches   int64
		}
		agg := qry.Copy()
		if err := agg.Group("channel_id").Column("channel_id").ColumnExpr("count(*) as matches").Select(&res); err != nil {
			return errwrap.Wrap(err, "Error performing aggregate search")
		}
		for _, c := range res {
			response.Aggs.Buckets[c.ChannelID] = c.Matches
		}
	}

	if val := ctx.r.FormValue("sort"); val == "asc" {
		qry.Order("timestamp ASC")
	} else {
		qry.Order("timestamp DESC")
	}

	if searchQuery != "" {
		qry.ColumnExpr(
			`jsonb_set(?TableAlias.msg, '{text}', ts_headline(?TableAlias.msg->'text', websearch_to_tsquery(?), 'StartSel=[hl] StopSel=[/hl] HighlightAll=true')) AS msg`,
			searchQuery,
		)
	}

	pager := models.NewPager(ctx.r.Form)

	pager.MaxLimit = 500
	qry = qry.Apply(pager.Pagination)

	qry = qry.Relation("User")

	if response.TotalCount, err = qry.SelectAndCount(); err != nil {
		return errwrap.Wrap(err, "Error selecting messages")
	}

	r := regexp.MustCompile(`\<\@(.+?)\>`)
	response.Messages = make([]slack.Message, 0, len(messages))

	// UserIDs to find
	userids := make(map[string]struct{})

	for _, message := range messages {
		response.Messages = append(response.Messages, *message.Msg)

		response.Related.Users[message.User.ID] = *message.User
		// If another message asked for this user, we've got it
		delete(userids, message.User.ID)

		// extract matches from message text
		func() {
			var matches [][]string
			if matches = r.FindAllStringSubmatch(message.Msg.Text, -1); matches == nil {
				return
			}

			for _, match := range matches {
				if _, ok := response.Related.Users[match[1]]; ok {
					// Already loaded from prefetch
					continue
				}
				userids[match[1]] = struct{}{}
			}
		}()

		if message.Msg.ParentUserId != "" {
			if _, ok := response.Related.Users[message.Msg.ParentUserId]; ok == false {
				userids[message.Msg.ParentUserId] = struct{}{}
			}
		}
	}

	users := []models.User{}

	if len(userids) == 0 {
		log.Debug("No non-prefetched users to query")
	} else {
		ids := make([]string, len(userids))
		i := 0
		for u := range userids {
			ids[i] = u
			i++
		}
		if err := api.session.Model((*models.User)(nil)).Where("id IN (?)", pg.In(ids)).Select(&users); err != nil {
			return errwrap.Wrap(err, "Error selectiing related users for message")
		}
	}

	for _, user := range users {
		response.Related.Users[user.ID] = user
	}

	//ctx.w.Header().Set("Content-Type", "application/json")
	return ctx.Write(response)
}

func (api *api) health(ctx *Context) error {
	ctx.Write("Approaching Neutral Zone, all systems normal and functioning.")
	return nil
}

func hash(s string) string {
	h := sha1.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// serveWs handles websocket requests from the peer.
func (api *api) serveWs(w http.ResponseWriter, r *http.Request) {
	if auth := r.Header.Get("Authorization"); auth == "" {
		w.WriteHeader(403)
		return
	} else if !strings.HasPrefix(auth, "Token") {
		w.WriteHeader(403)
		return
	} else if strings.Compare(auth[6:], hash(api.config.Bot.Token)) != 0 {
		w.WriteHeader(403)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("Error upgrading connection:", err)
		return
	}

	c := &connection{send: make(chan []byte, 256), ws: ws, api: api}
	defer c.Close()

	api.register <- c
	log.Infof("Connection upgraded: %s", ws.RemoteAddr())

	go c.readPump()
	c.writePump()
}

func (api *api) Serve() {
	r := mux.NewRouter()

	r.HandleFunc("/health.html", api.ContextHandlerFunc(api.health)).Methods("GET")

	sr := r.PathPrefix("/v1").Subrouter()

	sr.HandleFunc("/messages", api.ContextHandlerFunc(api.messagesHandler)).Methods("GET")
	sr.HandleFunc("/channels", api.ContextHandlerFunc(api.channelsHandler)).Methods("GET")
	sr.HandleFunc("/users", api.ContextHandlerFunc(api.usersHandler)).Methods("GET")
	sr.HandleFunc("/team", api.ContextHandlerFunc(api.teamHandler)).Methods("GET")
	/*
		api.HandleFunc("/messages", messagesHandler).Methods("GET")
		api.HandleFunc("/me", meHandler).Methods("GET")
	*/
	sr.HandleFunc("/oauth/login", api.ContextHandlerFunc(api.oAuthLoginHandler)).Methods("GET")
	sr.HandleFunc("/oauth/callback", api.ContextHandlerFunc(api.oAuthCallbackHandler)).Methods("GET")

	// run websocket server
	go api.run()
	//go api.indexer()

	r.HandleFunc("/ws", api.serveWs)

	sh := http.FileServer(
		AssetFS(),
	)

	r.PathPrefix("/").Handler(sh)
	r.NotFoundHandler = sh

	var handler http.Handler = r

	// install middlewares
	handler = handlers.LoggingHandler(handler)
	handler = handlers.RecoverHandler(handler)
	handler = handlers.RedirectHandler(handler)
	handler = handlers.CorsHandler(handler)

	// disable rate limiter for now
	// handler = ratelimit.Request(ratelimit.IP).Rate(30, 60*time.Second).LimitBy(memory.New())(sr)

	httpAddr := api.config.Listen

	log.Infof("SlackArchive server started. %v", httpAddr)
	log.Info("---------------------------")

	if httpsAddr := api.config.ListenTLS; httpsAddr != "" {
		go func() {
			m := autocert.Manager{
				Prompt: autocert.AcceptTOS,
				Cache:  autocert.DirCache(path.Join(api.config.Data, "cache")),
				HostPolicy: func(_ context.Context, host string) error {
					found := true

					/*
						for _, h := range []string{"slackarchive.io"} {
							found = found || strings.HasSuffix(host, h)
						}
					*/

					if !found {
						return errors.New("acme/autocert: host not configured")
					}

					return nil
				},
			}

			handler = m.HTTPHandler(handler)

			// SSL
			s := &http.Server{
				Addr:    httpsAddr,
				Handler: handler,
				TLSConfig: &tls.Config{
					GetCertificate: m.GetCertificate,
				},
			}

			if err := s.ListenAndServeTLS("", ""); err != nil {
				panic(err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)

	signal.Notify(stop, os.Interrupt)

	h := &http.Server{Addr: httpAddr, Handler: handler}

	go func() {
		if err := h.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe %s: %v", httpAddr, err)
		}
	}()

	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.Shutdown(ctx)

	//mg.Wait()
}
