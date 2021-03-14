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

func TestColly(t *testing.T) {
	ts := MockHTTP(`<html><head><script src="jquery-3.5.1.min.js"></script></head></html>`)
	defer ts.Close()
	config := NewConfig()
	config.Scraper = "colly"
	wapp, err := Init(config)
	res, err := wapp.Analyze(ts.URL)
	if err != nil {
		log.Errorln(err)
		t.FailNow()
	}
	log.Println(res)
}

func TestJSEval(t *testing.T) {
	ts := MockHTTP(`<html><head><script>jQuery=[];jQuery.fn=[];jQuery.fn.jquery="1.11.3"</script></head></html>`)
	defer ts.Close()
	config := NewConfig()
	wapp, err := Init(config)
	res, err := wapp.Analyze(ts.URL)
	if err != nil {
		log.Errorln(err)
		t.FailNow()
	}
	log.Println(res)
}

func TestDomSearch(t *testing.T) {
	ts := MockHTTP(`<html><head><body><a href='https://amzn.to'>Link<a/><div id='jira'></div></body></head></html>`)
	defer ts.Close()
	config := NewConfig()
	wapp, err := Init(config)
	res, err := wapp.Analyze(ts.URL)
	if err != nil {
		log.Errorln(err)
		t.FailNow()
	}
	log.Println(res)
}

func MockHTTP(content string) *httptest.Server {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, content)
			}))
	return ts
}
