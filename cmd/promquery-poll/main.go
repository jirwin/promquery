package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jirwin/promquery"
	"gopkg.in/urfave/cli.v2"
)

func PollerAction(c *cli.Context) error {
	if !c.IsSet("addr") {
		return cli.Exit("--addr is required", -1)
	}
	addr := c.String("addr")

	queries := c.StringSlice("query")

	ctx := context.Background()
	p, err := promquery.NewPoller([]string{addr}, queries)
	if err != nil {
		return cli.Exit(fmt.Sprintf("error creating poller: %s %#v", err.Error(), queries), -1)
	}

	p.Init(ctx)
	ctx1, _ := context.WithTimeout(ctx, time.Duration(c.Int("timeout"))*time.Second)
	err = <-p.Wait(ctx1, time.Duration(c.Int("interval"))*time.Second)
	if err != nil {
		return cli.Exit(fmt.Sprintf("error polling metrics: %s", err.Error()), -1)
	}
	return nil
}

func main() {
	app := &cli.App{
		Name:  "promquery-poll",
		Usage: "Grab a set of metrics from prometheus, and block until they've returned to normal.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "addr",
				Usage: "The prometheus server to query",
			},
			&cli.StringSliceFlag{
				Name:  "query",
				Usage: "A prometheus query to check. Specify multiple --query options for multiple queries",
			},
			&cli.IntFlag{
				Name:  "interval",
				Usage: "The number of seconds to wait before checking metrics (defaults to 30)",
				Value: 30,
			},
			&cli.IntFlag{
				Name:  "timeout",
				Usage: "The number of seconds to wait before failing",
				Value: 120,
			},
		},
		Action: PollerAction,
	}

	err := app.Run(os.Args)

	if err != nil {
		fmt.Println("error while polling:", err.Error())
		os.Exit(-1)
	}
}
