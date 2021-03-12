package core

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestGowap(t *testing.T) {
	url := "https://tengrinews.kz"
	config := NewConfig()
	config.JSON = false
	wapp, err := Init(config)
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

func TestBadUrl(t *testing.T) {
	url := "https://doesnotexist"
	wapp, err := Init(NewConfig())
	_, err = wapp.Analyze(url)
	if err == nil {
		log.Errorln(err)
		t.FailNow()
	}
}

func TestLoadingTimeout(t *testing.T) {
	ts := MockHTTP("<html><script>var now = Date.now();var end = now + 2000;while (now < end) { now = Date.now(); }</script></html>")
	defer ts.Close()
	config := NewConfig()
	config.PageLoadTimeoutSeconds = 1
	wapp, err := Init(config)
	_, err = wapp.Analyze(ts.URL)
	if err != nil {
		log.Errorln(err)
		t.FailNow()
	}
}

func MockHTTP(content string) *httptest.Server {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, content)
			}))
	return ts
}
