package scraper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollyScraper(t *testing.T) {
	scraperTest := &CollyScraper{}

	assert.False(t, scraperTest.CanRenderPage(), "Colly cannot render JS")
	_, err := scraperTest.EvalJS("jQuery")
	assert.Error(t, err, "Colly cannot render JS")

	err = scraperTest.Init()
	assert.NoError(t, err, "Scraper Init error")

	res, err := scraperTest.Scrape("https://tengrinews.kz")
	assert.NoError(t, err, "Colly scraping error")
	assert.NotEmpty(t, res.HTML, "There should be some HTML content")
}

func TestRodScraper(t *testing.T) {
	scraperTest := &RodScraper{BrowserTimeoutSeconds: 4, NetworkTimeoutSeconds: 2, PageLoadTimeoutSeconds: 2}

	assert.True(t, scraperTest.CanRenderPage(), "Rod can render JS")

	err := scraperTest.Init()
	assert.NoError(t, err, "Scraper Init error")

	res, err := scraperTest.Scrape("https://tengrinews.kz")
	assert.NoError(t, err, "Colly scraping error")
	assert.NotEmpty(t, res.HTML, "There should be some HTML content")

	resJS, err := scraperTest.EvalJS(`"test"`)
	assert.Equal(t, "test", *resJS, "Test string should eval as test string...")
	assert.NoError(t, err, "Rod should render JS")
	resJS, err = scraperTest.EvalJS("this.should.throw.error")
	assert.Nil(t, resJS, "Should return nil")
	assert.Error(t, err, "Rod should throw error on rendering bad JS")

	scraperTest.BrowserTimeoutSeconds = 0
	err = scraperTest.Init()
	assert.Error(t, err, "GoWap Init should throw an error")
	scraperTest.BrowserTimeoutSeconds = 4

	ts := MockHTTP("<html><script>var now = Date.now();var end = now + 2000;while (now < end) { now = Date.now(); }</script></html>")
	defer ts.Close()
	scraperTest.PageLoadTimeoutSeconds = 1
	err = scraperTest.Init()
	assert.NoError(t, err, "GoWap Init error")
	_, err = scraperTest.Scrape(ts.URL)
	assert.Error(t, err, "Timeout should throw error")
	scraperTest.PageLoadTimeoutSeconds = 2

	url := "https://doesnotexist"
	err = scraperTest.Init()
	assert.NoError(t, err, "GoWap Init error")
	_, err = scraperTest.Scrape(url)
	assert.Error(t, err, "Bad URL should throw error")

	ts = MockHTTP(`<html><head><meta property="generator" content="TiddlyWiki" /></head><body><div></div></body></html>`)
	defer ts.Close()
	res, err = scraperTest.Scrape(ts.URL)
	if assert.NoError(t, err, "Scrap should work") {
		assert.Equal(t, "TiddlyWiki", res.Meta["generator"][0], "Scrap meta should work")
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
