package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/fatih/color"
	"github.com/jzelinskie/geddit"
	"github.com/skratchdot/open-golang/open"
	"github.com/texttheater/golang-levenshtein/levenshtein"
	cli "gopkg.in/urfave/cli.v2"
)

// App information and constants
const (
	AppName        = "hnreader"
	AppVersion     = "v1.1"
	AppAuthor      = "Bunchhieng Soth"
	AppEmail       = "Bunchhieng@gmail.com"
	AppDescription = "Open multiple tech news feeds in your favorite browser through the command line."
	HackerNewsURL  = "https://news.ycombinator.com/news?p="
	LobstersURL    = "https://lobste.rs"
	DZoneURL       = "http://feeds.dzone.com/home"
	DevToURL       = "https://dev.to/feed"
)

// Supported operating systems (GOOS)
const (
	OSDarwin  = "darwin"
	OSLinux   = "linux"
	OSWindows = "windows"
)

// Colors for console output
var blue = color.New(color.FgBlue, color.Bold).SprintFunc()
var yellow = color.New(color.FgYellow, color.Bold).SprintFunc()
var red = color.New(color.FgRed, color.Bold).SprintFunc()

// Rss decode RSS xml
type Rss struct {
	Item []RssItem `xml:"channel>item"`
}

// RssItem item with link to news
type RssItem struct {
	Link string `xml:"link"`
}

type logWriter struct{}

// App contains author information
type App struct {
	Name, Version, Email, Description, Author string
}

// Fetcher retrieves stories from a source.
type Fetcher interface {
	Fetch(count int) (map[int]string, error)
}

// HackerNewsSource fetches new stories from news.ycombinator.com.
type HackerNewsSource struct{}

// Fetch gets news from the HackerNews
func (hn *HackerNewsSource) Fetch(count int) (map[int]string, error) {
	news := make(map[int]string)
	// 30 news per page
	pages := count / 30
	for i := 0; i <= pages; i++ {
		resp, err := http.Get(HackerNewsURL + strconv.Itoa(pages))
		if err != nil {
			handleError(err)
			continue
		}

		doc, err := goquery.NewDocumentFromReader(resp.Body)
		if err != nil {
			handleError(err)
			continue
		}

		doc.Find("a.storylink").Each(func(i int, s *goquery.Selection) {
			href, exist := s.Attr("href")
			if !exist {
				fmt.Println(red("can't find any stories..."))
			}
			news[i] = href
		})

		resp.Body.Close()
	}

	return news, nil
}

// RedditSource fetches new stories from reddit.com/r/programming.
type RedditSource struct{}

// Fetch gets news from the Reddit
func (rs *RedditSource) Fetch(count int) (map[int]string, error) {
	news := make(map[int]string)

	s := geddit.NewSession(fmt.Sprintf("desktop:com.github.Bunchhieng.%s:%s", AppName, AppVersion))
	subs, err := s.SubredditSubmissions(
		"programming",
		geddit.HotSubmissions,
		geddit.ListingOptions{
			Count: count,
			Limit: count,
		},
	)

	if err != nil {
		return news, err
	}

	for i, sub := range subs {
		news[i] = sub.URL
	}

	return news, nil
}

// LobstersSource fetches new stories from https://lobste.rs
type LobstersSource struct{}

// Fetch gets news from the Lobsters
func (l *LobstersSource) Fetch(count int) (map[int]string, error) {
	offset := float64(count) / float64(25)
	pages := int(math.Ceil(offset))
	news := make(map[int]string)
	newsIndex := 0

	for p := 1; p <= pages; p++ {
		url := fmt.Sprintf("%s/page/%d", LobstersURL, p)
		resp, err := http.Get(url)
		if err != nil {
			handleError(err)
			continue
		}

		doc, err := goquery.NewDocumentFromReader(resp.Body)
		if err != nil {
			handleError(err)
			continue
		}

		doc.Find(".link a.u-url").Each(func(_ int, s *goquery.Selection) {
			href, exist := s.Attr("href")
			if !exist {
				fmt.Println(red("can't find any stories..."))
			}

			if newsIndex >= count {
				return
			}

			// if internal link
			if strings.HasPrefix(href, "/") {
				href = LobstersURL + href
			}

			news[newsIndex] = href
			newsIndex++
		})

		resp.Body.Close()
	}

	return news, nil
}

// DZoneSource fetches latest stories from http://feeds.dzone.com/home
type DZoneSource struct{}

// Fetch gets news from the DZone
func (l *DZoneSource) Fetch(count int) (map[int]string, error) {
	news := make(map[int]string)

	resp, err := http.Get(DZoneURL)
	if err != nil {
		return news, err
	}

	defer resp.Body.Close()

	doc := Rss{}
	d := xml.NewDecoder(resp.Body)

	if err := d.Decode(&doc); err != nil {
		return news, err
	}

	for i, item := range doc.Item {
		if i >= count {
			break
		}

		news[i] = item.Link
	}

	return news, nil
}

// DevToSource fetches latest stories from https://dev.to/
type DevToSource struct{}

// Fetch gets news from the Dev.To
func (l *DevToSource) Fetch(count int) (map[int]string, error) {
	news := make(map[int]string)

	resp, err := http.Get(DevToURL)
	if err != nil {
		return news, err
	}

	defer resp.Body.Close()

	doc := Rss{}
	d := xml.NewDecoder(resp.Body)

	if err := d.Decode(&doc); err != nil {
		return news, err
	}

	for i, item := range doc.Item {
		if i >= count {
			break
		}

		news[i] = item.Link
	}

	return news, nil
}

// Init initializes the app
func Init() *App {
	return &App{
		Name:        AppName,
		Version:     AppVersion,
		Description: AppDescription,
		Author:      AppAuthor,
		Email:       AppEmail,
	}
}

// Information prints out app information
func (app *App) Information() {
	fmt.Println(blue(app.Name) + " - " + blue(app.Version))
	fmt.Println(blue(app.Description) + "\n")
}

func (writer logWriter) Write(bytes []byte) (int, error) {
	return fmt.Print(yellow("[") + time.Now().UTC().Format("15:04:05") + yellow("]") + string(bytes))
}

//RunApp opens a browser with input tabs count
func RunApp(tabs int, browser string, src Fetcher) error {
	news, err := src.Fetch(tabs)
	handleError(err)

	browser = findBrowser(browser)

	// To store the keys in slice in sorted order
	var keys []int
	for k := range news {
		keys = append(keys, k)
	}
	// Sort map keys
	sort.Ints(keys)

	for _, k := range keys {
		if k == tabs {
			break
		}

		var err error
		if browser == "" {
			fmt.Println(red("Trying default browser..."))
			err = open.Run(news[k])
		} else {
			err = open.RunWith(news[k], browser)
			if err != nil {
				fmt.Printf(red("%s is not found on this computer, trying default browser...\n"), browser)
				err = open.Run(news[k])
			}
		}

		if err != nil {
			os.Exit(1)
		}
	}
	return nil
}

func findBrowser(target string) string {
	if target == "" {
		return ""
	}
	browsers := []string{"google", "chrome", "mozilla", "firefox", "brave"}
	shortest := -1
	word := ""
	for _, browser := range browsers {
		distance := levenshtein.DistanceForStrings([]rune(browser), []rune(target), levenshtein.DefaultOptions)
		if distance == 0 {
			word = browser
			break
		}
		if distance <= shortest || shortest < 0 {
			shortest = distance
			word = browser
		}
	}

	return getBrowserNameByOS(word, runtime.GOOS)
}

func getGoogleChromeNameForOS(os string) string {
	switch os {
	case OSDarwin:
		return "Google Chrome"
	case OSLinux:
		return "google-chrome"
	case OSWindows:
		return "chrome"
	}
	return ""
}

func getFirefoxNameForOS(os string) string {
	switch os {
	case OSDarwin:
		return "Firefox"
	case OSLinux:
		return "firefox"
	case OSWindows:
		return "firefox"
	}
	return ""
}

func getBraveNameForOS(os string) string {
	switch os {
	case OSDarwin:
		return "Brave"
	case OSLinux:
		return "brave"
	case OSWindows:
		return "brave"
	}
	return ""
}

// getBrowserNameByOS normilizes browser name
func getBrowserNameByOS(browserFromCLI, os string) string {
	switch browserFromCLI {
	case "google", "chrome":
		return getGoogleChromeNameForOS(os)
	case "mozilla", "firefox":
		return getFirefoxNameForOS(os)
	case "brave":
		return getBraveNameForOS(os)
	}
	return ""
}

// checkGoPath checks for GOPATH
func checkGoPath() error {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		log.Fatal(red("$GOPATH isn't set up properly..."))
	}
	return nil
}

// handleError go convention
func handleError(err error) error {
	if err != nil {
		fmt.Println(red(err.Error()))
	}
	return nil
}

func init() {
	log.SetFlags(0)
	log.SetOutput(new(logWriter))
}

func main() {
	app := Init()

	cli := &cli.App{
		Name:    app.Name,
		Version: app.Version,
		Authors: []*cli.Author{
			{
				Name:  app.Author,
				Email: app.Email,
			},
		},
		Usage: app.Description,
		Commands: []*cli.Command{
			{
				Name:    "run",
				Aliases: []string{"r"},
				Usage:   "Start hnreader with default option (10 news and chrome browser)",
				Flags: []cli.Flag{
					&cli.UintFlag{
						Name:    "tabs",
						Value:   10,
						Aliases: []string{"t"},
						Usage:   "Specify number of tabs\t",
					},
					&cli.StringFlag{
						Name:    "browser",
						Value:   "",
						Aliases: []string{"b"},
						Usage:   "Specify browser\t",
					},
					&cli.StringFlag{
						Name:    "source",
						Value:   "hn",
						Aliases: []string{"s"},
						Usage:   "Specify news source (one of \"hn\", \"reddit\", \"lobsters\", \"dzone\", \"devto\")\t",
					},
				},
				Action: func(c *cli.Context) error {
					var src Fetcher

					switch c.String("source") {
					case "hn", "hackernews":
						src = new(HackerNewsSource)
					case "reddit":
						src = new(RedditSource)
					case "lobsters":
						src = new(LobstersSource)
					case "dzone":
						src = new(DZoneSource)
					case "devto":
						src = new(DevToSource)
					default:
						return handleError(fmt.Errorf("invalid source: %s", c.String("source")))
					}

					return handleError(RunApp(c.Int("tabs"), c.String("browser"), src))
				},
				Before: func(c *cli.Context) error {
					app.Information()
					checkGoPath()
					return nil
				},
			},
		},
	}

	cli.Run(os.Args)
}
