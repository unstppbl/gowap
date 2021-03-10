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
	flag.StringVar(&url, "url", "", "URL to analyse")
	flag.StringVar(&appsJSONPath, "file", "", "Path to override default technologies.json file")
	flag.Parse()

	if len(url) == 0 {
		fmt.Println("You must specify a url to analyse with -url")
		fmt.Println("Usage : gowap")
		flag.PrintDefaults()
		os.Exit(1)
	}

	wapp, err := gowap.Init(appsJSONPath, false)
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
