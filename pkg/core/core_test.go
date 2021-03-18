package core

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err, "GoWap Init error")
	_, err = wapp.Analyze(url)
	assert.Error(t, err, "Bad URL should throw error")
}

func TestLoadingTimeout(t *testing.T) {
	ts := MockHTTP("<html><script>var now = Date.now();var end = now + 2000;while (now < end) { now = Date.now(); }</script></html>")
	defer ts.Close()
	config := NewConfig()
	config.PageLoadTimeoutSeconds = 1
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	_, err = wapp.Analyze(ts.URL)
	assert.Error(t, err, "Timeout should throw error")
}

func TestColly(t *testing.T) {
	ts := MockHTTP(`<html><head><script src="jquery-3.5.1.min.js"></script></head></html>`)
	defer ts.Close()
	config := NewConfig()
	config.Scraper = "colly"
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	res, err := wapp.Analyze(ts.URL)
	assert.NoError(t, err, "GoWap Analyze error")
	var output output
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")

	//We should have jquery in the output
	var expected technology
	for _, v := range output.Technologies {
		if v.Name == "jQuery" {
			expected = v
		}
	}
	assert.Equal(t, "3.5.1", expected.Version, "We should find jQuery version 3.5.1")
}

func TestJSEval(t *testing.T) {
	ts := MockHTTP(`<html><head></head><script>jQuery=[];jQuery.fn=[];jQuery.fn.jquery="1.11.3"</script></html>`)
	defer ts.Close()
	config := NewConfig()
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	res, err := wapp.Analyze(ts.URL)
	assert.NoError(t, err, "GoWap Analyze error")
	var output output
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")

	//We should have jquery in the output
	var expected technology
	for _, v := range output.Technologies {
		if v.Name == "jQuery" {
			expected = v
		}
	}
	assert.Equal(t, "1.11.3", expected.Version, "We should find jQuery version 1.11.3")
}

func TestDomSearch(t *testing.T) {
	ts := MockHTTP(`<html><head></head><body><a href='https://amzn.to'>Link<a/><div id='jira'></div></body></html>`)
	defer ts.Close()
	config := NewConfig()
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	res, err := wapp.Analyze(ts.URL)
	assert.NoError(t, err, "GoWap Analyze error")
	var output output
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")

	var found bool
	for _, v := range output.Technologies {
		if v.Name == "Atlassian Jira" {
			found = true
		}
	}
	assert.True(t, found, "Atlassian Jira should be found in DOM")
}

func TestLoadExternalTechnologiesJSON(t *testing.T) {
	config := NewConfig()
	config.AppsJSONPath = "assets/technologies.json"
	_, err := Init(config)
	assert.NoError(t, err, "External JSON technologies file should open")

	config.AppsJSONPath = "assets/nofile.json"
	_, err = Init(config)
	assert.NoError(t, err, "Should load internal JSON if file not present")
}

func TestTechnologiesFileParsing(t *testing.T) {
	//Bad file format
	config := NewConfig()
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	unmarshalError := []byte(`{"this":"is","notgood"}`)
	err = parseTechnologiesFile(&unmarshalError, wapp)
	assert.Error(t, err, "Unmarshalling should throw an error")
	unmarshalCategoriesError := []byte(`{"categories":{"is":"notgood"}}`)
	err = parseTechnologiesFile(&unmarshalCategoriesError, wapp)
	assert.Error(t, err, "Unmarshalling Categories should throw an error")
	unmarshalAppsError := []byte(`{"categories":{"1":{"name":"CMS","priority":1}},"technologies":{"is":"notgood"}}`)
	err = parseTechnologiesFile(&unmarshalAppsError, wapp)
	assert.Error(t, err, "Unmarshalling Apps should throw an error")
	noCategoryFound := []byte(`{"this":"isgood"}`)
	err = parseTechnologiesFile(&noCategoryFound, wapp)
	assert.Error(t, err, "Should throw an error NoCategoryFound")
	noTechnologyFound := []byte(`{"categories":{"1":{"name":"CMS","priority":1}},"this":"isgood"}`)
	err = parseTechnologiesFile(&noTechnologyFound, wapp)
	assert.Error(t, err, "Should throw an error NoTechnologyFound")

	//Error loading included asset
	embedPath = "does/not/exist"
	_, err = Init(config)
	assert.Error(t, err, "GoWap Init should throw an error trying to open non existing embed file")
	embedPath = "assets/technologies.json"

}

func TestImpliesExcludes(t *testing.T) {
	ts := MockHTTP(`<html><head></head><body><script>Drupal="test"; Backdrop="test";</script><div></div></body></html>`)
	defer ts.Close()
	config := NewConfig()
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	res, err := wapp.Analyze(ts.URL)
	assert.NoError(t, err, "GoWap Analyze error")
	var output output
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")

	var found bool
	for _, v := range output.Technologies {
		assert.NotEqual(t, "AngularDart", v.Name, "Backdrop should exclude Drupal")
		if v.Name == "PHP" {
			found = true
		}
	}
	assert.True(t, found, "Backdrop should imply PHP")
}

func TestMeta(t *testing.T) {
	ts := MockHTTP(`<html><head><meta name="generator" content="TiddlyWiki" /></head><body><div></div></body></html>`)
	defer ts.Close()
	config := NewConfig()
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	res, err := wapp.Analyze(ts.URL)
	assert.NoError(t, err, "GoWap Analyze error")
	var output output
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")

	var found bool
	for _, v := range output.Technologies {
		if v.Name == "TiddlyWiki" {
			found = true
		}
	}
	assert.True(t, found, "TiddlyWiki should be find in meta")
}

func TestUrl(t *testing.T) {
	config := NewConfig()
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	res, err := wapp.Analyze("https://twitter.github.io/")
	assert.NoError(t, err, "GoWap Analyze error")
	var output output
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")

	var found bool
	for _, v := range output.Technologies {
		if v.Name == "GitHub Pages" {
			found = true
		}
	}
	assert.True(t, found, "GitHub Pages should be find in URL")
}

func TestHTML(t *testing.T) {
	ts := MockHTTP(`<html><head><title>RoundCube</title></head><body><div></div></body></html>`)
	defer ts.Close()
	config := NewConfig()
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	res, err := wapp.Analyze(ts.URL)
	assert.NoError(t, err, "GoWap Analyze error")
	var output output
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")

	var found bool
	for _, v := range output.Technologies {
		if v.Name == "RoundCube" {
			found = true
		}
	}
	assert.True(t, found, "RoundCube should be find in HTML")
}

func TestUnkownScraper(t *testing.T) {
	config := NewConfig()
	config.Scraper = "Unknown"
	_, err := Init(config)
	assert.Error(t, err, "Should throw an error")
}

func TestConfidence(t *testing.T) {
	ts := MockHTTP(`<html><head><script src="alpine.min.js"></script></head><body><div x-data="dropdown()"></div></body></html>`)
	defer ts.Close()
	config := NewConfig()
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	res, err := wapp.Analyze(ts.URL)
	assert.NoError(t, err, "GoWap Analyze error")
	var output output
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")
	var found bool
	for _, v := range output.Technologies {
		if v.Name == "Alpine.js" {
			assert.Equal(t, 100, v.Confidence, "Alpine.js confidence should be 100")
			found = true
		}
	}
	assert.True(t, found, "AOS should be found")
}

func TestVersion(t *testing.T) {
	ts := MockHTTP(`<html><head><script src="4.5.6/modernizr.1.2.3.js"></script></head><body><div></div></body></html>`)
	defer ts.Close()
	config := NewConfig()
	wapp, err := Init(config)
	assert.NoError(t, err, "GoWap Init error")
	res, err := wapp.Analyze(ts.URL)
	assert.NoError(t, err, "GoWap Analyze error")
	var output output
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")

	var found bool
	for _, v := range output.Technologies {
		if v.Name == "Modernizr" {
			assert.Equal(t, "4.5.6", v.Version, "Modernizr version should be 4.5.6")
			found = true
		}
	}
	assert.True(t, found, "Modernizr should be found")

	ts2 := MockHTTP(`<html><head><script src="abc/modernizr.1.2.3.js"></script></head><body><div></div></body></html>`)
	defer ts2.Close()
	res, err = wapp.Analyze(ts2.URL)
	assert.NoError(t, err, "GoWap Analyze error")
	err = json.UnmarshalFromString(res.(string), &output)
	assert.NoError(t, err, "Unmarshal error")
	for _, v := range output.Technologies {
		if v.Name == "Modernizr" {
			assert.Equal(t, "1.2.3", v.Version, "Modernizr version should be 1.2.3")
			found = true
		}
	}
	assert.True(t, found, "Modernizr should be found")
}

func TestParsePattern(t *testing.T) {
	patterns := make(map[string]int)
	//Logging output should be tested here
	parsePatterns(patterns)
	patterns2 := make(map[string]interface{})
	patterns2["test"] = patterns
	parsePatterns(patterns2)
}

func TestBrowserInit(t *testing.T) {
	config := NewConfig()
	config.BrowserTimeoutSeconds = 0
	_, err := Init(config)
	assert.Error(t, err, "GoWap Init should throw an error")
}

func MockHTTP(content string) *httptest.Server {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, content)
			}))
	return ts
}
