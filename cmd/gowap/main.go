package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	gowap "github.com/bernardmendy/gowap/pkg/core"
	log "github.com/sirupsen/logrus"
)

func main() {

	var url string
	var appsJSONPath string
	var help bool
	var browserTimeoutSeconds, networkTimeoutSeconds, pageLoadTimeoutSeconds int
	var rawOutput bool
	flag.StringVar(&appsJSONPath, "file", "", "Path to override default technologies.json file")
	flag.IntVar(&browserTimeoutSeconds, "bwtimeout", 4, "Timeout in seconds for browser")
	flag.IntVar(&networkTimeoutSeconds, "nttimeout", 3, "Timeout in seconds for the network connection to the url")
	flag.IntVar(&pageLoadTimeoutSeconds, "pgtimeout", 3, "Timeout in seconds for the page loading by the browser")
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

	config := gowap.NewConfig()
	config.AppsJSONPath = appsJSONPath
	config.JSON = !rawOutput
	config.BrowserTimeoutSeconds = browserTimeoutSeconds
	config.NetworkTimeoutSeconds = networkTimeoutSeconds
	config.PageLoadTimeoutSeconds = pageLoadTimeoutSeconds

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
