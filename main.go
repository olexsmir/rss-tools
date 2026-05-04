package main

import (
	"context"
	"flag"

	"go.etcd.io/bbolt"
	"olexsmir.xyz/rss-tools/app"
	"olexsmir.xyz/rss-tools/sources/moviefeed"
	"olexsmir.xyz/rss-tools/sources/telegram"
	"olexsmir.xyz/rss-tools/sources/weather"
	"olexsmir.xyz/rss-tools/sources/ztoe"
)

func main() {
	var cfgPath, dbPath string
	flag.StringVar(&cfgPath, "config", "./config.json", "Path to config file")
	flag.StringVar(&dbPath, "db", "./db", "Path to local database")
	flag.Parse()

	if err := run(context.Background(), cfgPath, dbPath); err != nil {
		panic(err)
	}
}

func run(ctx context.Context, cfgPath, dbPath string) error {
	db, err := bbolt.Open(dbPath, 0o600, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	cfg, err := app.NewConfig(cfgPath)
	if err != nil {
		return err
	}

	app := app.New(cfg, db)
	_ = ztoe.Register(app)
	_ = telegram.Register(app)
	_ = moviefeed.Register(app)
	_ = weather.Register(app)

	return app.Start(ctx)
}
