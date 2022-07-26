package main

import (
	"fmt"
	"github.com/go-pg/pg/orm"
	"math/rand"
	"os"
	_ "os/exec"
	"time"

	cli "gopkg.in/urfave/cli.v1"

	"github.com/ashb/slackarchive/api"
	"github.com/ashb/slackarchive/bot"
	"github.com/ashb/slackarchive/config"
	"github.com/ashb/slackarchive/importer"
	"github.com/ashb/slackarchive/models"
	"github.com/go-pg/migrations"
	"github.com/go-pg/pg"
	"github.com/op/go-logging"

	_ "github.com/ashb/slackarchive/migrations"
)

var log = logging.MustGetLogger("main")

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

var version = "0.1"

func main() {
	app := cli.NewApp()
	app.Name = "slack-archive"
	app.Version = version
	app.Flags = append(app.Flags, []cli.Flag{
		cli.StringFlag{
			Name:   "config, c",
			Value:  "config.yaml",
			Usage:  "Custom configuration file path",
			EnvVar: "",
		},
	}...)
	app.Commands = []cli.Command{
		{
			Name:        "run",
			Action:      run,
			Description: "Run webserver and Slack bot user",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name: "debug, D",
				},
			},
		},
		{
			Name:   "import",
			Action: doImport,
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name: "debug, D",
				},
			},
			ArgsUsage: "xoxb-bot-token /path/to/data",
			UsageText: "Provided data folder should have contain a folder matching\n" +
				"   the domain for the token. For example a myteam.slack.com data/ should\n" +
				"   contain myteam/",
		},
		{
			Name: "migrate",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name: "debug, D",
				},
			},
			Subcommands: []cli.Command{
				cli.Command{
					Name: "up", Usage: "runs all available migrations, or up to specified version",
					Action:    migrate,
					ArgsUsage: "[VERSION]",
				},
				cli.Command{
					Name: "down", Usage: "reverts last migration",
					Action: migrate,
				},
				cli.Command{
					Name: "init", Usage: "initialize migrations tables",
					Action: migrate,
				},
				cli.Command{
					Name: "reset", Usage: "reverts all migrations",
					Action: migrate,
				},
				cli.Command{
					Name: "version", Usage: "prints current db version",
					Action: migrate,
				},
				cli.Command{
					Name: "set_version", Usage: "sets db version without running migrations", ArgsUsage: "VERSION",
					Action: migrate,
				},
				cli.Command{
					Name: "create", Hidden: true,
					Action: migrate,
				},
			},
		},
		{
			Name: "init",
			Action: initArchive,
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name: "debug, D",
				},
			},
		},
	}

	app.Run(os.Args)
}

func run(c *cli.Context) error {
	conf, err := config.Load(c.GlobalString("config"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	db, err := models.Connect(conf.Database.DSN, c.Bool("debug"))
	if err != nil {
		return err
	}

	api := api.New(conf, db)
	bot := bot.New(conf, db)
	bot.Start()
	api.Serve()
	return nil
}

func doImport(c *cli.Context) error {
	if c.NArg() != 2 {
		cli.ShowCommandHelpAndExit(c, c.Command.FullName(), 1)

	}
	conf, err := config.Load(c.GlobalString("config"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	i := importer.New(conf, c.Bool("debug"))
	i.Import(c.Args().Get(0), c.Args().Get(1))
	return nil
}

func migrate(c *cli.Context) error {
	config, err := config.Load(c.GlobalString("config"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	opts, err := pg.ParseURL(config.Database.DSN)
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	db := pg.Connect(opts)
	if c.Bool("debug") || c.Parent().Bool("debug") {
		db.AddQueryHook(models.DBLogger{Logger: log})
	}

	if c.Command.Name == "create" {
		if err := os.Chdir("migrations"); err != nil {
			return cli.NewExitError(err, 1)
		}
	}

	args := []string{c.Command.Name}
	args = append(args, c.Args()...)

	if oldVersion, newVersion, err := migrations.Run(db, args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	} else if newVersion != oldVersion {
		fmt.Printf("migrated from version %d to %d\n", oldVersion, newVersion)
	} else {
		fmt.Printf("version is %d\n", oldVersion)
	}

	return nil
}

func initArchive(c *cli.Context) error {
	if err := initDb(c); err != nil {
		return err
	}

	return firstRetrieve(c)
}

func initDb(c *cli.Context) error {
	conf, err := config.Load(c.GlobalString("config"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	db, err := models.Connect(conf.Database.DSN, c.Bool("debug"))
	if err != nil {
		return err
	}

	err = db.Model(&models.Team{}).CreateTable(&orm.CreateTableOptions{IfNotExists: true})
	if err != nil {
		return err
	}

	err = db.Model(&models.User{}).CreateTable(&orm.CreateTableOptions{IfNotExists: true})
	if err != nil {
		return err
	}

	err = db.Model(&models.Channel{}).CreateTable(&orm.CreateTableOptions{IfNotExists: true})
	if err != nil {
		return err
	}

	err = db.Model(&models.Message{}).CreateTable(&orm.CreateTableOptions{IfNotExists: true})
	if err != nil {
		return err
	}

	return nil
}

func firstRetrieve(c *cli.Context) error {
	conf, err := config.Load(c.GlobalString("config"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	db, err := models.Connect(conf.Database.DSN, c.Bool("debug"))
	if err != nil {
		return err
	}

	bot := bot.New(conf, db)
	bot.RetrieveAll()
	return nil
}