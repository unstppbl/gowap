package core

import (
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestGowap(t *testing.T) {
	url := "https://tengrinews.kz"
	wapp, err := Init("", false)
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
