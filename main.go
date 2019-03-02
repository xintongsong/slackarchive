package main

import (
	"fmt"
	"math/rand"
	"os"
	_ "os/exec"
	"time"

	cli "gopkg.in/urfave/cli.v1"

	"github.com/dutchcoders/slackarchive/api"
	"github.com/dutchcoders/slackarchive/config"
	"github.com/dutchcoders/slackarchive/importer"
	"github.com/dutchcoders/slackarchive/models"
	"github.com/go-pg/migrations"
	"github.com/go-pg/pg"
	"github.com/op/go-logging"

	_ "github.com/dutchcoders/slackarchive/migrations"
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
			Description: "Run webserver",
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
	}

	app.Run(os.Args)
}

func run(c *cli.Context) {
	conf := config.MustLoad(c.GlobalString("config"))

	api := api.New(conf)
	api.Serve()
}

func doImport(c *cli.Context) {
	if c.NArg() != 2 {
		cli.ShowCommandHelpAndExit(c, c.Command.FullName(), 1)

	}
	conf := config.MustLoad(c.GlobalString("config"))
	i := importer.New(conf, c.Bool("debug"))
	i.Import(c.Args().Get(0), c.Args().Get(1))
}

func migrate(c *cli.Context) {
	config := config.MustLoad(c.GlobalString("config"))

	opts, err := pg.ParseURL(config.Database.DSN)
	if err != nil {
		panic(err)
	}
	db := pg.Connect(opts)
	if c.Bool("debug") || c.Parent().Bool("debug") {
		fmt.Println("debug")
		db.AddQueryHook(models.DBLogger{Logger: log})
	} else {
		fmt.Println("ndebug")
	}

	migrations.DefaultCollection.DisableSQLAutodiscover(true)

	if c.Command.Name == "create" {
		if err := os.Chdir("migrations"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
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
}
