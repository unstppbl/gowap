package gowap

import (
	"fmt"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/extensions"
)

func initLogger() {
	Formatter := new(log.TextFormatter)
	Formatter.TimestampFormat = time.RFC1123Z
	Formatter.FullTimestamp = true
	log.SetFormatter(Formatter)
	log.SetOutput(os.Stdout)
}

// Wappalyzer func identifies technologies on provided web-resource
func Wappalyzer(url string) {
	initLogger()

	c := colly.NewCollector(
		colly.IgnoreRobotsTxt(),
		colly.MaxDepth(2),
		// colly.Debugger(&debug.LogDebugger{}),
	)
	extensions.RandomUserAgent(c)
	extensions.Referrer(c)

	c.OnRequest(func(r *colly.Request) {
		log.Infof("Visiting %s", r.URL)
		log.Infof("User-agent: %s", r.Headers)
	})

	c.OnError(func(_ *colly.Response, err error) {
		log.Error(err)
	})

	c.OnResponse(func(r *colly.Response) {
		log.Infof("Visited %s", r.Request.URL)
		log.Infof("Status code: %d", r.StatusCode)
		fmt.Println(string(r.Body))
	})

	c.OnHTML("script", func(e *colly.HTMLElement) {
		fmt.Println(e.Text)
		fmt.Println(e.Attr("src"))
	})
	c.Visit(url)
}
