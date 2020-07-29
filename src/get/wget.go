package get

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/cretz/bine/tor"
	"github.com/pkg/errors"
	"github.com/schollz/fbdb"
	log "github.com/schollz/logger"
	"github.com/schollz/pluck/pluck"
	"github.com/schollz/progressbar/v2"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/html"
)

var minifier *minify.M

func init() {
	minifier = minify.New()
	minifier.Add("text/html", &html.Minifier{
		KeepDefaultAttrVals: true,
		KeepDocumentTags:    true,
	})
}

func (w *Get) Run() (err error) {
	if w.NumWorkers < 1 {
		return errors.New("cannot have less than 1 worker")
	}
	return w.start()
}

type Get struct {
	Debug           bool
	DBName          string
	UseTor          bool
	NoClobber       bool
	FileWithList    string
	URL             string
	Cookies         string
	Headers         []string
	CompressResults bool
	NumWorkers      int
	PluckerTOML     string
	StripCSS        bool
	StripJS         bool
	torconnection   []*tor.Tor
	fs              *fbdb.FileSystem
	bar             *progressbar.ProgressBar
}

type job struct {
	URL string
}
type result struct {
	URL string
	err error
}

func New(g Get) (w *Get, err error) {
	w = new(Get)
	w.Debug = g.Debug
	w.DBName = g.DBName
	if w.DBName == "" {
		w.DBName = "urls.db"
	}
	w.UseTor = g.UseTor
	w.NoClobber = g.NoClobber
	w.URL = g.URL
	w.FileWithList = g.FileWithList
	w.Cookies = g.Cookies
	w.CompressResults = g.CompressResults
	w.NumWorkers = g.NumWorkers
	w.PluckerTOML = g.PluckerTOML
	w.StripCSS = g.StripCSS
	w.StripJS = g.StripJS
	w.Headers = g.Headers
	return
}

func (w *Get) getURL(id int, jobs <-chan job, results chan<- result) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 20,
		},
		Timeout: 10 * time.Second,
	}
	defer func() {
		log.Tracef("worker %d finished", id)
	}()

RestartTor:
	if w.UseTor {
		// keep trying until it gets on
		for {
			log.Tracef("starting tor in worker %d", id)
			// Wait at most a minute to start network and get
			dialCtx, dialCancel := context.WithTimeout(context.Background(), 3000*time.Hour)
			defer dialCancel()
			// Make connection
			dialer, err := w.torconnection[id].Dialer(dialCtx, nil)
			if err != nil {
				log.Warn(err)
				continue
			}
			httpClient.Transport = &http.Transport{
				DialContext:         dialer.DialContext,
				MaxIdleConnsPerHost: 20,
			}

			// Get /
			resp, err := httpClient.Get("http://icanhazip.com/")
			if err != nil {
				log.Warn(err)
				continue
			}

			body, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Warn(err)
				continue
			}
			log.Tracef("worker %d IP: %s", id, bytes.TrimSpace(body))
			break
		}
	}

	for j := range jobs {
		err := func(j job) (err error) {
			// check if valid url
			if !strings.Contains(j.URL, "://") {
				j.URL = "https://" + j.URL
			}

			filename := strings.Split(j.URL, "://")[1]
			// if no clobber, check if exists
			if w.NoClobber {
				var exists bool
				exists, err = w.fs.Exists(filename)
				if err != nil {
					return
				}
				if exists {
					log.Debugf("already saved %s", j.URL)
					return nil
				}
			}

			// make request
			req, err := http.NewRequest("GET", j.URL, nil)
			if err != nil {
				err = errors.Wrap(err, "bad request")
				return
			}
			if len(w.Headers) > 0 {
				for _, header := range w.Headers {
					if strings.Contains(header, ":") {
						hs := strings.Split(header, ":")
						req.Header.Set(strings.TrimSpace(hs[0]), strings.TrimSpace(hs[1]))
					}
				}
			}
			if w.Cookies != "" {
				req.Header.Set("Cookie", w.Cookies)
			}
			resp, err := httpClient.Do(req)
			if err != nil && resp == nil {
				err = errors.Wrap(err, "bad do")
				return
			}

			// check request's validity
			log.Tracef("%d requested %s: %d %s", id, j.URL, resp.StatusCode, http.StatusText(resp.StatusCode))
			if resp.StatusCode == 503 || resp.StatusCode == 403 {
				err = fmt.Errorf("received %d code", resp.StatusCode)
				if w.UseTor {
					err = errors.Wrap(err, "restart tor")
				}
				return
			}

			// read out body
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return
			}

			if w.StripCSS || w.StripJS {
				doc, errD := goquery.NewDocumentFromReader(bytes.NewReader(body))
				if errD != nil {
					err = errD
					return
				}

				if w.StripCSS {
					doc.Find("style").ReplaceWithHtml("")
				}
				if w.StripJS {
					doc.Find("script").ReplaceWithHtml("")
				}
				html, errD := doc.Html()
				if errD != nil {
					err = errD
					return
				}

				s, errD := minifier.String("text/html", html)
				if errD != nil {
					err = errD
					return
				}
				body = []byte(s)
			}
			if w.PluckerTOML != "" {
				plucker, _ := pluck.New()
				r := bufio.NewReader(bytes.NewReader(body))
				err = plucker.LoadFromString(w.PluckerTOML)
				if err != nil {
					return err
				}
				err = plucker.PluckStream(r)
				if err != nil {
					return
				}
				body = []byte(plucker.ResultJSON())
				log.Tracef("body: %s", body)
				if !bytes.Contains(body, []byte("{")) {
					return fmt.Errorf("could not get anything")
				}
			}

			// save
			f, err := w.fs.NewFile(filename, body)
			if err != nil {
				return
			}
			err = w.fs.Save(f)
			if err == nil {
				log.Debugf("saved %s", j.URL)
			}
			return
		}(j)
		results <- result{
			URL: j.URL,
			err: err,
		}
		if err != nil && strings.Contains(err.Error(), "restart tor") {
			goto RestartTor
		}
	}
}

func (w *Get) cleanup(interrupted bool) {
	if w.UseTor {
		for i := range w.torconnection {
			err := w.torconnection[i].Close()
			if err != nil {
				log.Errorf("problem closing tor connection %d: %s", i, err.Error())
			}
		}

		torFolders, err := filepath.Glob("data-dir-*")
		if err != nil {
			log.Error(err)
			return
		}
		for _, torFolder := range torFolders {
			errRemove := os.RemoveAll(torFolder)
			if errRemove == nil {
				log.Tracef("removed %s", torFolder)
			}
		}
	}

	if w.fs != nil {
		w.fs.Close()
	}

	if interrupted {
		os.Exit(1)
	}
}

func (w *Get) start() (err error) {
	log.Infof("saving urls to '%s'", w.DBName)
	defer w.cleanup(false)

	log.Tracef("starting with params: %+v", w)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			// cleanup
			log.Trace(sig)
			w.cleanup(true)
		}
	}()

	log.Tracef("opening %s", w.DBName)
	w.fs, err = fbdb.Open(w.DBName, fbdb.OptionCompress(w.CompressResults))
	if err != nil {
		panic(err)
	}

	if w.UseTor {
		w.torconnection = make([]*tor.Tor, w.NumWorkers)
		for i := 0; i < w.NumWorkers; i++ {
			w.torconnection[i], err = tor.Start(nil, nil)
			if err != nil {
				return
			}
		}
	}

	numURLs := 1
	if w.FileWithList != "" {
		log.Trace("counting number of lines")
		numURLs, err = countLines(w.FileWithList)
		if err != nil {
			return
		}
		log.Tracef("found %d lines", numURLs)
	}

	if !w.Debug {
		w.bar = progressbar.NewOptions64(int64(numURLs),
			progressbar.OptionShowIts(),
			progressbar.OptionShowCount(),
		)
	}

	jobs := make(chan job, numURLs)
	results := make(chan result, numURLs)

	for i := 0; i < w.NumWorkers; i++ {
		go w.getURL(i, jobs, results)
	}

	// submit jobs
	numJobs := 1
	if w.FileWithList != "" {
		var file *os.File
		file, err = os.Open(w.FileWithList)
		if err != nil {
			return
		}

		scanner := bufio.NewScanner(file)
		numJobs = 0
		for scanner.Scan() {
			numJobs++
			jobs <- job{
				URL: strings.TrimSpace(scanner.Text()),
			}
		}
		log.Tracef("sent %d jobs", numJobs)

		if errScan := scanner.Err(); errScan != nil {
			log.Error(errScan)
		}
		file.Close()
	} else {
		jobs <- job{
			URL: w.URL,
		}
	}
	close(jobs)

	// print out errors
	log.Tracef("waiting for %d jobs", numJobs)
	for i := 0; i < numJobs; i++ {
		if !w.Debug {
			w.bar.Add(1)
		}
		a := <-results
		if a.err != nil {
			log.Warnf("problem with %s: %s", a.URL, a.err.Error())
		}
	}
	w.bar.Finish()

	return
}
