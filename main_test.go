package gowap

import (
	"encoding/json"
	"testing"

	log "github.com/Sirupsen/logrus"
	"github.com/altsab/gowap"
)

func TestGowap(t *testing.T) {
	url := "https://tengrinews.kz"
	wapp, err := gowap.Init("./apps.json", false)
	if err != nil {
		log.Errorln(err)
		t.FailNow()
	}
	res, err := wapp.Analyze(url)
	if err != nil {
		log.Errorln(err)
		t.FailNow()
	}
	prettyJSON, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		log.Errorln(err)
		t.FailNow()
	}
	log.Infof("[*] Result for %s:\n%s", url, string(prettyJSON))
}
