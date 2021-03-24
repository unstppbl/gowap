package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	gowap "github.com/dranih/gowap/pkg/core"
	log "github.com/sirupsen/logrus"
)

func main() {

	var url, appsJSONPath, scraper string
	var help, rawOutput bool
	var timeoutSeconds, loadingTimeoutSeconds, maxDepth, maxVisitedLinks, msDelayBetweenRequests int
	flag.StringVar(&appsJSONPath, "file", "", "Path to override default technologies.json file")
	flag.StringVar(&scraper, "scraper", "rod", "Choose scraper between rod (default) and colly")
	flag.IntVar(&timeoutSeconds, "timeout", 3, "Timeout in seconds for fetching the url")
	flag.IntVar(&loadingTimeoutSeconds, "loadtimeout", 3, "Timeout in seconds for loading the page")
	flag.IntVar(&maxDepth, "depth", 0, "Don't analyze page when depth superior to this number. Default (0) means no recursivity (only first page will be analyzed)")
	flag.IntVar(&maxVisitedLinks, "maxlinks", 5, "Max number of pages to visit. Exit when reached")
	flag.IntVar(&msDelayBetweenRequests, "delay", 100, "Delay in ms between requests")
	flag.BoolVar(&rawOutput, "raw", false, "Raw output (JSON by default)")
	flag.BoolVar(&help, "h", false, "Help")
	flag.Parse()

	var Usage = func() {
		fmt.Println("Usage : gowap [options] <url>")
		flag.PrintDefaults()
	}

	if help {
		Usage()
		os.Exit(1)
	}
	if flag.NArg() == 0 {
		fmt.Println("You must specify a url to analyse")
		Usage()
		os.Exit(1)
	} else if flag.NArg() > 1 {
		fmt.Printf("Too many arguments %s", flag.Args())
		Usage()
		os.Exit(1)
	} else {
		url = flag.Arg(0)
	}
	if scraper != "rod" && scraper != "colly" {
		fmt.Printf("Unknown scraper %s : only supporting rod and colly", scraper)
		Usage()
		os.Exit(1)
	}

	config := gowap.NewConfig()
	config.AppsJSONPath = appsJSONPath
	config.JSON = !rawOutput
	config.TimeoutSeconds = timeoutSeconds
	config.LoadingTimeoutSeconds = loadingTimeoutSeconds
	config.MaxDepth = maxDepth
	config.MaxVisitedLinks = maxVisitedLinks
	config.MsDelayBetweenRequests = msDelayBetweenRequests
	config.Scraper = scraper

	wapp, err := gowap.Init(config)
	if err != nil {
		log.Errorln(err)
	}
	res, err := wapp.Analyze(url)
	if err != nil {
		log.Errorln(err)
	}
	prettyJSON, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		log.Errorln(err)
	}
	log.Infof("[*] Result for %s:\n%s", url, string(prettyJSON))
}
