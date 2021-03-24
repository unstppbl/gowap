package core

import (
	"embed"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	scraper "github.com/dranih/gowap/pkg/scraper"
	log "github.com/sirupsen/logrus"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary
var wg sync.WaitGroup

//go:embed assets/technologies.json
var f embed.FS
var embedPath = "assets/technologies.json"

// Config for gowap
type Config struct {
	AppsJSONPath           string
	TimeoutSeconds         int
	LoadingTimeoutSeconds  int
	JSON                   bool
	Scraper                string
	MaxDepth               int
	visitedLinks           int
	MaxVisitedLinks        int
	MsDelayBetweenRequests int
}

// NewConfig struct with default values
func NewConfig() *Config {
	return &Config{
		AppsJSONPath:           "",
		TimeoutSeconds:         3,
		LoadingTimeoutSeconds:  3,
		JSON:                   true,
		Scraper:                "rod",
		MaxDepth:               0,
		visitedLinks:           0,
		MaxVisitedLinks:        10,
		MsDelayBetweenRequests: 100,
	}
}

type temp struct {
	Apps       map[string]*jsoniter.RawMessage `json:"technologies"`
	Categories map[string]*jsoniter.RawMessage `json:"categories"`
}

type application struct {
	Name       string   `json:"name,omitempty"`
	Version    string   `json:"version"`
	Categories []string `json:"categories,omitempty"`

	Cats     []int                             `json:"cats,omitempty"`
	Cookies  interface{}                       `json:"cookies,omitempty"`
	Dom      map[string]map[string]interface{} `json:"dom,omitempty"`
	Js       interface{}                       `json:"js,omitempty"`
	Headers  interface{}                       `json:"headers,omitempty"`
	HTML     interface{}                       `json:"html,omitempty"`
	Excludes interface{}                       `json:"excludes,omitempty"`
	Implies  interface{}                       `json:"implies,omitempty"`
	Meta     interface{}                       `json:"meta,omitempty"`
	Scripts  interface{}                       `json:"scripts,omitempty"`
	DNS      interface{}                       `json:"dns,omitempty"`
	URL      string                            `json:"url,omitempty"`
	Website  string                            `json:"website,omitempty"`
}

type category struct {
	Name     string `json:"name,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// Wappalyzer implements analyze method as original wappalyzer does
type Wappalyzer struct {
	Scraper    scraper.Scraper
	Apps       map[string]*application
	Categories map[string]*category
	Config     *Config
}

// Init initializes wappalyzer
func Init(config *Config) (wapp *Wappalyzer, err error) {
	wapp = &Wappalyzer{Config: config}
	// Scraper initialization
	switch config.Scraper {
	case "colly":
		wapp.Scraper = &scraper.CollyScraper{TimeoutSeconds: config.TimeoutSeconds, LoadingTimeoutSeconds: config.LoadingTimeoutSeconds}
		err = wapp.Scraper.Init()
	case "rod":
		wapp.Scraper = &scraper.RodScraper{TimeoutSeconds: config.TimeoutSeconds, LoadingTimeoutSeconds: config.LoadingTimeoutSeconds}
		err = wapp.Scraper.Init()
	default:
		log.Errorf("Unknown scraper %s", config.Scraper)
		err = errors.New("UnknownScraper")
	}

	if err != nil {
		log.Errorf("Scraper %s initialization failed : %v", config.Scraper, err)
		return nil, err
	}

	var appsFile []byte
	if config.AppsJSONPath != "" {
		log.Infof("Trying to open technologies file at %s", config.AppsJSONPath)
		appsFile, err = ioutil.ReadFile(config.AppsJSONPath)
		if err != nil {
			log.Warningf("Couldn't open file at %s\n", config.AppsJSONPath)
		} else {
			log.Infof("Technologies file opened")
		}
	}
	if config.AppsJSONPath == "" || len(appsFile) == 0 {
		log.Infof("Loading included asset %s", embedPath)
		appsFile, err = f.ReadFile(embedPath)
		if err != nil {
			log.Errorf("Couldn't open included asset %s\n", embedPath)
			return nil, err
		}
	}

	err = parseTechnologiesFile(&appsFile, wapp)
	return wapp, err
}

func parseTechnologiesFile(appsFile *[]byte, wapp *Wappalyzer) error {
	temporary := &temp{}
	err := json.Unmarshal(*appsFile, &temporary)
	if err != nil {
		log.Errorf("Couldn't unmarshal apps.json file: %s\n", err)
		return err
	}
	wapp.Apps = make(map[string]*application)
	wapp.Categories = make(map[string]*category)
	for k, v := range temporary.Categories {
		catg := &category{}
		if err = json.Unmarshal(*v, catg); err != nil {
			log.Errorf("[!] Couldn't unmarshal Categories: %s\n", err)
			return err
		}
		wapp.Categories[k] = catg
	}
	if len(wapp.Categories) < 1 {
		log.Errorf("Couldn't find categories in technologies file")
		return errors.New("NoCategoryFound")
	}
	for k, v := range temporary.Apps {
		app := &application{}
		app.Name = k
		if err = json.Unmarshal(*v, app); err != nil {
			log.Errorf("Couldn't unmarshal Apps: %s\n", err)
			return err
		}
		parseCategories(app, &wapp.Categories)
		wapp.Apps[k] = app
	}
	if len(wapp.Apps) < 1 {
		log.Errorf("Couldn't find technologies in technologies file")
		return errors.New("NoTechnologyFound")
	}
	return err
}

type resultApp struct {
	technology technology
	excludes   interface{}
	implies    interface{}
}

type technology struct {
	Name       string   `json:"name,omitempty"`
	Version    string   `json:"version"`
	Categories []string `json:"categories,omitempty"`
	Confidence int      `json:"confidence"`
}

type detected struct {
	Mu   *sync.Mutex
	Apps map[string]*resultApp
}

type output struct {
	URLs         []scraper.ScrapedURL `json:"urls,omitempty"`
	Technologies []technology         `json:"technologies,omitempty"`
}

func (wapp *Wappalyzer) Analyze(paramURL string) (result interface{}, err error) {
	detectedApplications := &detected{new(sync.Mutex), make(map[string]*resultApp)}
	toVisitURLs := make(map[string]struct{})
	globalVisitedURLs := make(map[string]scraper.ScrapedURL)
	err = errors.New("analyzePageFailed")

	paramURL = strings.TrimRight(paramURL, "/")
	toVisitURLs[paramURL] = struct{}{}
	for depth := 0; depth <= wapp.Config.MaxDepth; depth++ {
		log.Printf("Depth : %d", depth)
		links, visitedURLs, retErr := analyzePages(toVisitURLs, wapp, detectedApplications)
		//If we have at least one page ok => no error
		if err != nil && retErr == nil {
			err = nil
		}

		for visitedURL, result := range visitedURLs {
			globalVisitedURLs[visitedURL] = result
		}
		if depth < wapp.Config.MaxDepth {
			toVisitURLs = make(map[string]struct{})
			for link := range links {
				if _, exists := globalVisitedURLs[link]; !exists {
					toVisitURLs[link] = struct{}{}
				}
			}
		}
	}
	if err == nil {
		res := &output{}
		for _, visited := range globalVisitedURLs {
			res.URLs = append(res.URLs, visited)
		}
		for _, app := range detectedApplications.Apps {
			res.Technologies = append(res.Technologies, app.technology)
		}
		if wapp.Config.JSON {
			return json.MarshalToString(res)
		}
		return res, err
	} else {
		return nil, err
	}
}

func analyzePages(paramURLs map[string]struct{}, wapp *Wappalyzer, detectedApplications *detected) (detectedLinks map[string]struct{}, visitedURLs map[string]scraper.ScrapedURL, err error) {
	visitedURLs = make(map[string]scraper.ScrapedURL)
	detectedLinks = make(map[string]struct{})
	err = errors.New("AnalyzePageFailed")
	for paramURL := range paramURLs {
		links, scrapedURL, retErr := analyzePage(paramURL, wapp, detectedApplications)
		//If we have at least one page ok => no error
		if err != nil && retErr == nil {
			err = nil
		}
		if scrapedURL != nil {
			visitedURLs[paramURL] = *scrapedURL
			if links != nil {
				for link := range *links {
					if _, exists := detectedLinks[link]; !exists {
						detectedLinks[link] = struct{}{}
					}
				}
			}
		}
		wapp.Config.visitedLinks = wapp.Config.visitedLinks + 1
		if wapp.Config.visitedLinks >= wapp.Config.MaxVisitedLinks {
			log.Printf("Visited max number of pages : %d", wapp.Config.MaxVisitedLinks)
			break
		}
		time.Sleep(time.Duration(wapp.Config.MsDelayBetweenRequests) * time.Millisecond)
	}
	return detectedLinks, visitedURLs, err
}

// Analyze retrieves application stack used on the provided web-site
func analyzePage(paramURL string, wapp *Wappalyzer, detectedApplications *detected) (links *map[string]struct{}, scrapedURL *scraper.ScrapedURL, err error) {
	log.Printf("Analyzing %s", paramURL)
	if !validateURL(paramURL) {
		log.Errorf("URL not valid : %s", paramURL)
		return nil, &scraper.ScrapedURL{URL: paramURL, Status: 400}, errors.New("UrlNotValid")
	}

	scraped, err := wapp.Scraper.Scrape(paramURL)
	if err != nil {
		log.Errorf("Scraper failed : %v", err)
		return nil, &scraper.ScrapedURL{URL: paramURL, Status: 400}, err
	}

	canRenderPage := wapp.Scraper.CanRenderPage()
	reader := strings.NewReader(scraped.HTML)
	doc, err := goquery.NewDocumentFromReader(reader)
	if err == nil {
		links = getLinksSlice(doc, paramURL)
	}

	for _, app := range wapp.Apps {
		wg.Add(1)
		go func(app *application) {
			defer wg.Done()
			analyzeURL(app, paramURL, detectedApplications)
			if canRenderPage && app.Js != nil {
				analyzeJS(app, wapp.Scraper, detectedApplications)
			}
			if canRenderPage && app.Dom != nil {
				analyzeDom(app, doc, detectedApplications)
			}
			if app.HTML != nil {
				analyzeHTML(app, scraped.HTML, detectedApplications)
			}
			if len(scraped.Headers) > 0 && app.Headers != nil {
				analyzeHeaders(app, scraped.Headers, detectedApplications)
			}
			if len(scraped.Cookies) > 0 && app.Cookies != nil {
				analyzeCookies(app, scraped.Cookies, detectedApplications)
			}
			if len(scraped.Scripts) > 0 && app.Scripts != nil {
				analyzeScripts(app, scraped.Scripts, detectedApplications)
			}
			if len(scraped.Meta) > 0 && app.Meta != nil {
				analyzeMeta(app, scraped.Meta, detectedApplications)
			}
			if len(scraped.DNS) > 0 && app.DNS != nil {
				analyzeDNS(app, scraped.DNS, detectedApplications)
			}
		}(app)
	}

	wg.Wait()

	for _, app := range detectedApplications.Apps {
		if app.excludes != nil {
			resolveExcludes(&detectedApplications.Apps, app.excludes)
		}
		if app.implies != nil {
			resolveImplies(&wapp.Apps, &detectedApplications.Apps, app.implies)
		}
	}
	return links, &scraped.URLs, nil
}

func analyzeURL(app *application, paramURL string, detectedApplications *detected) {
	patterns := parsePatterns(app.URL)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil && pattrn.regex.MatchString(paramURL) {
				version := detectVersion(pattrn, &paramURL)
				addApp(app, detectedApplications, version, pattrn.confidence)
			}
		}
	}
}

func analyzeScripts(app *application, scripts []string, detectedApplications *detected) {
	patterns := parsePatterns(app.Scripts)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil {
				for _, script := range scripts {
					if pattrn.regex.MatchString(script) {
						version := detectVersion(pattrn, &script)
						addApp(app, detectedApplications, version, pattrn.confidence)
					}
				}
			}
		}
	}
}

func analyzeHeaders(app *application, headers map[string][]string, detectedApplications *detected) {
	patterns := parsePatterns(app.Headers)
	for headerName, v := range patterns {
		headerNameLowerCase := strings.ToLower(headerName)
		for _, pattrn := range v {
			if headersSlice, ok := headers[headerNameLowerCase]; ok {
				for _, header := range headersSlice {
					if pattrn.str == "" || (pattrn.regex != nil && pattrn.regex.MatchString(header)) {
						version := detectVersion(pattrn, &header)
						addApp(app, detectedApplications, version, pattrn.confidence)
					}
				}
			}
		}
	}
}

func analyzeCookies(app *application, cookies map[string]string, detectedApplications *detected) {
	patterns := parsePatterns(app.Cookies)
	for cookieName, v := range patterns {
		cookieNameLowerCase := strings.ToLower(cookieName)
		for _, pattrn := range v {
			if cookie, ok := cookies[cookieNameLowerCase]; ok {
				if pattrn.str == "" || (pattrn.regex != nil && pattrn.regex.MatchString(cookie)) {
					version := detectVersion(pattrn, &cookie)
					addApp(app, detectedApplications, version, pattrn.confidence)
				}
			}
		}
	}
}

func analyzeHTML(app *application, html string, detectedApplications *detected) {
	patterns := parsePatterns(app.HTML)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil && pattrn.regex.MatchString(html) {
				version := detectVersion(pattrn, &html)
				addApp(app, detectedApplications, version, pattrn.confidence)
			}
		}

	}
}

func analyzeMeta(app *application, metas map[string][]string, detectedApplications *detected) {
	patterns := parsePatterns(app.Meta)
	for metaName, v := range patterns {
		metaNameLowerCase := strings.ToLower(metaName)
		for _, pattrn := range v {
			if metaSlice, ok := metas[metaNameLowerCase]; ok {
				for _, meta := range metaSlice {
					if pattrn.str == "" || (pattrn.regex != nil && pattrn.regex.MatchString(meta)) {
						version := detectVersion(pattrn, &meta)
						addApp(app, detectedApplications, version, pattrn.confidence)
					}
				}
			}
		}
	}
}

// analyzeJS evals the JS properties and tries to match
func analyzeJS(app *application, scraper scraper.Scraper, detectedApplications *detected) {
	patterns := parsePatterns(app.Js)
	for jsProp, v := range patterns {
		value, err := scraper.EvalJS(jsProp)
		if err == nil && value != nil {
			for _, pattrn := range v {
				if pattrn.str == "" || (pattrn.regex != nil && pattrn.regex.MatchString(*value)) {
					version := detectVersion(pattrn, value)
					addApp(app, detectedApplications, version, pattrn.confidence)
				}
			}
		}
	}
}

// analyzeDom evals the DOM tries to match
func analyzeDom(app *application, doc *goquery.Document, detectedApplications *detected) {
	for domSelector, v1 := range app.Dom {
		doc.Find(domSelector).First().Each(func(i int, s *goquery.Selection) {
			for domType, v := range v1 {
				patterns := parsePatterns(v)
				for attribute, pattrns := range patterns {
					for _, pattrn := range pattrns {
						var value string
						var exists bool
						switch domType {
						case "text":
							value = s.Text()
							exists = true
						case "properties":
							// Not implemented, should be done into the browser to get element properties
						case "attributes":
							value, exists = s.Attr(attribute)
						}
						if exists && pattrn.str == "" || (pattrn.regex != nil && pattrn.regex.MatchString(value)) {
							version := detectVersion(pattrn, &value)
							addApp(app, detectedApplications, version, pattrn.confidence)
						}
					}
				}
			}
		})
	}
}

// analyzeDNS tries to match dns records
func analyzeDNS(app *application, dns map[string][]string, detectedApplications *detected) {
	patterns := parsePatterns(app.DNS)
	for dnsType, v := range patterns {
		dnsTypeUpperCase := strings.ToUpper(dnsType)
		for _, pattrn := range v {
			if dnsSlice, ok := dns[dnsTypeUpperCase]; ok {
				for _, dns := range dnsSlice {
					if pattrn.str == "" || (pattrn.regex != nil && pattrn.regex.MatchString(dns)) {
						version := detectVersion(pattrn, &dns)
						addApp(app, detectedApplications, version, pattrn.confidence)
					}
				}
			}
		}
	}
}

// addApp add a detected app to the detectedApplications
// if the app is already detected, we merge it (version, confidence, ...)
func addApp(app *application, detectedApplications *detected, version string, confidence int) {
	detectedApplications.Mu.Lock()
	if _, ok := (*detectedApplications).Apps[app.Name]; !ok {
		resApp := &resultApp{technology{app.Name, version, app.Categories, confidence}, app.Excludes, app.Implies}
		(*detectedApplications).Apps[resApp.technology.Name] = resApp
	} else {
		if (*detectedApplications).Apps[app.Name].technology.Version == "" {
			(*detectedApplications).Apps[app.Name].technology.Version = version
		}
		if confidence > (*detectedApplications).Apps[app.Name].technology.Confidence {
			(*detectedApplications).Apps[app.Name].technology.Confidence = confidence
		}
	}
	detectedApplications.Mu.Unlock()
}

// detectVersion tries to extract version from value when app detected
func detectVersion(pattrn *pattern, value *string) (res string) {
	if pattrn.regex == nil {
		return ""
	}
	versions := make(map[string]interface{})
	version := pattrn.version
	if slices := pattrn.regex.FindAllStringSubmatch(*value, -1); slices != nil {
		for _, slice := range slices {
			for i, match := range slice {
				reg, _ := regexp.Compile(fmt.Sprintf("%s%d%s", "\\\\", i, "\\?([^:]+):(.*)$"))
				ternary := reg.FindStringSubmatch(version)
				if len(ternary) == 3 {
					if match != "" {
						version = strings.Replace(version, ternary[0], ternary[1], -1)
					} else {
						version = strings.Replace(version, ternary[0], ternary[2], -1)
					}
				}
				reg2, _ := regexp.Compile(fmt.Sprintf("%s%d", "\\\\", i))
				version = reg2.ReplaceAllString(version, match)
			}
		}
		if _, ok := versions[version]; !ok && version != "" {
			versions[version] = struct{}{}
		}
		if len(versions) != 0 {
			for ver := range versions {
				if ver > res {
					res = ver
				}
			}
		}
	}
	return res
}

type pattern struct {
	str        string
	regex      *regexp.Regexp
	version    string
	confidence int
}

func parsePatterns(patterns interface{}) (result map[string][]*pattern) {
	parsed := make(map[string][]string)
	switch ptrn := patterns.(type) {
	case string:
		parsed["main"] = append(parsed["main"], ptrn)
	case map[string]interface{}:
		for k, v := range ptrn {
			switch content := v.(type) {
			case string:
				parsed[k] = append(parsed[k], v.(string))
			case []interface{}:
				for _, v1 := range content {
					parsed[k] = append(parsed[k], v1.(string))
				}
			default:
				log.Errorf("Unkown type in parsePatterns: %T\n", v)
			}
		}
	case []interface{}:
		var slice []string
		for _, v := range ptrn {
			slice = append(slice, v.(string))
		}
		parsed["main"] = slice
	default:
		log.Errorf("Unkown type in parsePatterns: %T\n", ptrn)
	}
	result = make(map[string][]*pattern)
	for k, v := range parsed {
		for _, str := range v {
			appPattern := &pattern{confidence: 100}
			slice := strings.Split(str, "\\;")
			for i, item := range slice {
				if item == "" {
					continue
				}
				if i > 0 {
					additional := strings.SplitN(item, ":", 2)
					if len(additional) > 1 {
						if additional[0] == "version" {
							appPattern.version = additional[1]
						} else if additional[0] == "confidence" {
							appPattern.confidence, _ = strconv.Atoi(additional[1])
						}
					}
				} else {
					appPattern.str = item
					first := strings.Replace(item, `\/`, `/`, -1)
					second := strings.Replace(first, `\\`, `\`, -1)
					reg, err := regexp.Compile(fmt.Sprintf("%s%s", "(?i)", strings.Replace(second, `/`, `\/`, -1)))
					if err == nil {
						appPattern.regex = reg
					}
				}
			}
			result[k] = append(result[k], appPattern)
		}
	}
	return result
}

func resolveExcludes(detected *map[string]*resultApp, value interface{}) {
	patterns := parsePatterns(value)
	for _, v := range patterns {
		for _, excluded := range v {
			delete(*detected, excluded.str)
		}
	}
}

func resolveImplies(apps *map[string]*application, detected *map[string]*resultApp, value interface{}) {
	patterns := parsePatterns(value)
	for _, v := range patterns {
		for _, implied := range v {
			app, ok := (*apps)[implied.str]
			if _, ok2 := (*detected)[implied.str]; ok && !ok2 {
				resApp := &resultApp{technology{app.Name, implied.version, app.Categories, implied.confidence}, app.Excludes, app.Implies}
				(*detected)[implied.str] = resApp
				if app.Implies != nil {
					resolveImplies(apps, detected, app.Implies)
				}
			}
		}
	}
}

func parseCategories(app *application, categoriesCatalog *map[string]*category) {
	for _, categoryID := range app.Cats {
		app.Categories = append(app.Categories, (*categoriesCatalog)[strconv.Itoa(categoryID)].Name)
	}
}

func validateURL(paramURL string) bool {
	regex, err := regexp.Compile(`^(?:http(s)?:\/\/)?[\w.-]+(?:\.[\w\.-]+)+[\w\-\._~:/?#[\]@!\$&'\(\)\*\+,;=.]+$`)
	ret := false
	if err == nil {
		ret = regex.MatchString(paramURL)
	}
	return ret
}

// getLinksSlice parses query doc and return links
func getLinksSlice(doc *goquery.Document, currentURL string) *map[string]struct{} {
	ret := make(map[string]struct{})
	parsedCurrentURL, _ := url.Parse(currentURL)
	var protocolRegex = regexp.MustCompile(`^https?`)

	doc.Find("body a").Each(func(index int, item *goquery.Selection) {
		rawLink, _ := item.Attr("href")
		parsedLink, _ := url.Parse(rawLink)
		if parsedLink.Scheme == "" {
			parsedLink.Scheme = parsedCurrentURL.Scheme
		}
		if matched := protocolRegex.MatchString(parsedLink.Scheme); matched && (parsedLink.Host == "" || parsedLink.Host == parsedCurrentURL.Host) {
			ret[parsedLink.Scheme+"://"+parsedCurrentURL.Host+strings.TrimRight(parsedLink.Path, "/")] = struct{}{}
		}
	})
	return &ret
}
