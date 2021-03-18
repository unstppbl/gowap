package scraper

import (
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

}
