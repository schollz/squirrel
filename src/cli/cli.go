package cli

import (
	"errors"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/schollz/fbdb"
	log "github.com/schollz/logger"
	"github.com/schollz/progressbar/v2"
	"github.com/schollz/squirrel/src/get"
	"github.com/urfave/cli"
)

func init() {
	log.SetLevel("debug")
}

func Run() (err error) {

	app := cli.NewApp()
	app.Name = "squirrel"
	app.Version = "v1.0.0-59b5d52"
	app.Compiled = time.Now()
	app.Usage = "download URLs directly into an SQLite database"
	app.Flags = []cli.Flag{
		cli.StringSliceFlag{Name: "headers,H", Usage: "headers to include"},
		cli.BoolFlag{Name: "tor"},
		cli.BoolFlag{Name: "no-clobber,nc"},
		cli.StringFlag{Name: "list,i"},
		cli.StringFlag{Name: "pluck,p", Usage: "file for plucking"},
		cli.StringFlag{Name: "cookies,c"},
		cli.BoolFlag{Name: "compressed"},
		cli.BoolFlag{Name: "quiet,q"},
		cli.IntFlag{Name: "workers,w", Value: 1},
		cli.BoolFlag{Name: "dump", Usage: "dump database file to disk"},
		cli.StringFlag{Name: "db", Usage: "name of SQLite database to use", Value: "urls.db"},
		cli.BoolFlag{Name: "debug", Usage: "increase verbosity"},
	}
	app.Action = func(c *cli.Context) error {
		if c.GlobalBool("dump") {
			return dump(c)
		}
		return runget(c)
	}
	app.Before = func(c *cli.Context) error {
		if c.GlobalBool("debug") {
			log.SetLevel("debug")
		} else {
			log.SetLevel("warn")
		}
		return nil
	}

	// ignore error so we don't exit non-zero and break gfmrun README example tests
	return app.Run(os.Args)
}

func runget(c *cli.Context) (err error) {
	w := get.Get{}
	w.DBName = c.GlobalString("db")
	if c.Args().First() != "" {
		w.URL = c.Args().First()
	} else if c.GlobalString("list") != "" {
		w.FileWithList = c.GlobalString("list")
	} else {
		return errors.New("need to specify URL")
	}
	if c.GlobalBool("debug") {
		log.SetLevel("debug")
	} else if c.GlobalBool("quiet") {
		log.SetLevel("error")
	} else {
		log.SetLevel("info")
	}
	w.Headers = c.GlobalStringSlice("headers")
	w.NoClobber = c.GlobalBool("no-clobber")
	w.UseTor = c.GlobalBool("tor")
	w.CompressResults = c.GlobalBool("compressed")
	w.NumWorkers = c.GlobalInt("workers")
	w.Cookies = c.GlobalString("cookies")
	if w.NumWorkers < 1 {
		return errors.New("cannot have less than 1 worker")
	}
	if c.GlobalString("pluck") != "" {
		b, err := ioutil.ReadFile(c.GlobalString("pluck"))
		if err != nil {
			return err
		}
		w.PluckerTOML = string(b)
	}

	w2, _ := get.New(w)
	return w2.Run()
}

func dump(c *cli.Context) (err error) {
	_, err = os.Stat(c.GlobalString("db"))
	if err != nil {
		return
	}
	fs, err := fbdb.Open(c.GlobalString("db"))
	if err != nil {
		return
	}
	numFiles, err := fs.Len()
	if err != nil {
		return
	}
	bar := progressbar.NewOptions(numFiles,
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
	)
	for i := 0; i < numFiles; i++ {
		bar.Add(1)
		var f fbdb.File
		f, err = fs.GetI(i)
		if err != nil {
			return
		}
		pathname, filename := path.Split(strings.TrimSuffix(strings.TrimSpace(f.Name), "/"))
		os.MkdirAll(pathname, 0755)
		err = ioutil.WriteFile(path.Join(pathname, filename), f.Data, 0644)
		if err != nil {
			log.Error(err)
			continue
		}
	}
	return
}
