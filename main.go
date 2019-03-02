package main

import (
	"math/rand"
	"os"
	_ "os/exec"
	"time"

	cli "gopkg.in/urfave/cli.v1"

	"github.com/dutchcoders/slackarchive/api"
	"github.com/dutchcoders/slackarchive/config"
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
	app.Commands = []cli.Command{
		{
			Name:        "run",
			Action:      run,
			Description: "Run webserver",
		},
	}

	app.Run(os.Args)
}

func run(c *cli.Context) {
	conf := config.MustLoad(c.GlobalString("config"))

	api := api.New(conf)
	api.Serve()
}
