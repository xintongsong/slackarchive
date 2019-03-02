package main

import (
	"math/rand"
	"os"
	_ "os/exec"
	"time"

	cli "gopkg.in/urfave/cli.v1"

	"github.com/dutchcoders/slackarchive/api"
	"github.com/dutchcoders/slackarchive/config"
	"github.com/dutchcoders/slackarchive/importer"
	"github.com/go-pg/pg"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("main")

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

var version = "0.1"

func main() {
	app := cli.NewApp()
	app.Name = "SlackArchive"
	app.Version = version
	app.Flags = append(app.Flags, []cli.Flag{
		cli.StringFlag{
			Name:   "config, c",
			Value:  "config.yaml",
			Usage:  "Custom configuration file path",
			EnvVar: "",
		},
	}...)
	app.HelpName = "slack-archive"
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
