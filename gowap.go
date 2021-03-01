package gowap

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary
var wg sync.WaitGroup
var appMu sync.Mutex

type collyData struct {
	status  int
	url     string
	html    string
	headers map[string][]string
	scripts []string
	cookies map[string]string
	meta    map[string][]string
}

type temp struct {
	Apps       map[string]*jsoniter.RawMessage `json:"technologies"`
	Categories map[string]*jsoniter.RawMessage `json:"categories"`
}
type application struct {
	Name       string   `json:"name,ompitempty"`
	Version    string   `json:"version"`
	Categories []string `json:"categories,omitempty"`

	Cats     []int       `json:"cats,omitempty"`
	Cookies  interface{} `json:"cookies,omitempty"`
	Js       interface{} `json:"js,omitempty"`
	Headers  interface{} `json:"headers,omitempty"`
	HTML     interface{} `json:"html,omitempty"`
	Excludes interface{} `json:"excludes,omitempty"`
	Implies  interface{} `json:"implies,omitempty"`
	Meta     interface{} `json:"meta,omitempty"`
	Scripts  interface{} `json:"scripts,omitempty"`
	URL      string      `json:"url,omitempty"`
	Website  string      `json:"website,omitempty"`
}

type category struct {
	Name     string `json:"name,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// Wappalyzer implements analyze method as original wappalyzer does
type Wappalyzer struct {
	//Collector  *colly.Collector
	Apps       map[string]*application
	Categories map[string]*category
	JSON       bool
	Transport  *http.Transport
}

// Init initializes wappalyzer
func Init(appsJSONPath string, JSON bool) (wapp *Wappalyzer, err error) {
	wapp = &Wappalyzer{}
	wapp.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: time.Second * 90,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	}
	appsFile, err := ioutil.ReadFile(appsJSONPath)
	if err != nil {
		log.Errorf("Couldn't open file at %s\n", appsJSONPath)
		return nil, err
	}
	temporary := &temp{}
	err = json.Unmarshal(appsFile, &temporary)
	if err != nil {
		log.Errorf("Couldn't unmarshal apps.json file: %s\n", err)
		return nil, err
	}
	wapp.Apps = make(map[string]*application)
	wapp.Categories = make(map[string]*category)
	for k, v := range temporary.Categories {
		catg := &category{}
		if err = json.Unmarshal(*v, catg); err != nil {
			log.Errorf("[!] Couldn't unmarshal Categories: %s\n", err)
			return nil, err
		}
		wapp.Categories[k] = catg
	}
	for k, v := range temporary.Apps {
		app := &application{}
		app.Name = k
		if err = json.Unmarshal(*v, app); err != nil {
			log.Errorf("Couldn't unmarshal Apps: %s\n", err)
			return nil, err
		}
		parseCategories(app, &wapp.Categories)
		wapp.Apps[k] = app
	}
	wapp.JSON = JSON
	return wapp, nil
}

type resultApp struct {
	Name       string   `json:"name,ompitempty"`
	Version    string   `json:"version"`
	Categories []string `json:"categories,omitempty"`
	excludes   interface{}
	implies    interface{}
}

// Analyze retrieves application stack used on the provided web-site
func (wapp *Wappalyzer) Analyze(url string) (result interface{}, err error) {
	/*wapp.Collector = colly.NewCollector(
		colly.IgnoreRobotsTxt(),
	)
	wapp.Collector.WithTransport(wapp.Transport)

	extensions.Referer(wapp.Collector)
	extensions.RandomUserAgent(wapp.Collector)*/

	detectedApplications := make(map[string]*resultApp)
	scraped := &collyData{}

	/*wapp.Collector.OnResponse(func(r *colly.Response) {
		// log.Infof("Visited %s", r.Request.URL)
		scraped.headers = make(map[string][]string)
		for k, v := range *r.Headers {
			lowerCaseKey := strings.ToLower(k)
			scraped.headers[lowerCaseKey] = v
		}

		scraped.html = string(r.Body)

		scraped.cookies = make(map[string]string)
		for _, cookie := range scraped.headers["set-cookie"] {
			keyValues := strings.Split(cookie, ";")
			for _, keyValueString := range keyValues {
				keyValueSlice := strings.Split(keyValueString, "=")
				if len(keyValueSlice) > 1 {
					key, value := keyValueSlice[0], keyValueSlice[1]
					scraped.cookies[key] = value
				}
			}
		}
	})

	//Scrapping meta elements
	scraped.meta = make(map[string][]string)
	wapp.Collector.OnHTML("meta", func(e *colly.HTMLElement) {
		name := e.Attr(`name`)
		if name == "" {
			name = e.Attr(`property`)
		}
		scraped.meta[strings.ToLower(name)] = append(scraped.meta[name], e.Attr("content"))
	})

	wapp.Collector.OnHTML("script", func(e *colly.HTMLElement) {
		scraped.scripts = append(scraped.scripts, e.Attr("src"))
	})

	err = wapp.Collector.Visit(url)
	if err != nil {
		return nil, err
	}*/

	var e proto.NetworkResponseReceived
	page := rod.New().MustConnect().MustPage("")
	wait := page.WaitEvent(&e)
	go page.MustHandleDialog()
	page.MustNavigate(url)
	wait()

	scraped.status = e.Response.Status
	scraped.url = e.Response.URL
	scraped.headers = make(map[string][]string)
	for header, value := range e.Response.Headers {
		lowerCaseKey := strings.ToLower(header)
		scraped.headers[lowerCaseKey] = append(scraped.headers[lowerCaseKey], value.String())
	}

	scraped.html = page.MustWaitLoad().MustHTML()

	scripts := page.MustWaitLoad().MustElements("script")
	for _, script := range scripts {
		if src := script.MustProperty("src").String(); len(src) > 0 {
			scraped.scripts = append(scraped.scripts, src)
		}
	}

	metas := page.MustWaitLoad().MustElements("meta")
	scraped.meta = make(map[string][]string)
	for _, meta := range metas {
		name := strings.ToLower(meta.MustProperty("name").String())
		if len(name) <= 0 {
			name = strings.ToLower(meta.MustProperty("property").String())
		}
		scraped.meta[name] = append(scraped.meta[name], meta.MustProperty("content").String())
	}

	scraped.cookies = make(map[string]string)
	str := []string{}
	cookies, _ := page.MustWaitLoad().Cookies(str)
	for _, cookie := range cookies {
		scraped.cookies[cookie.Name] = cookie.Value
	}

	for _, app := range wapp.Apps {
		wg.Add(1)
		go func(app *application) {
			defer wg.Done()
			analyzeURL(app, url, &detectedApplications)
			if app.HTML != nil {
				analyzeHTML(app, scraped.html, &detectedApplications)
			}
			if len(scraped.headers) > 0 && app.Headers != nil {
				analyzeHeaders(app, scraped.headers, &detectedApplications)
			}
			if len(scraped.cookies) > 0 && app.Cookies != nil {
				analyzeCookies(app, scraped.cookies, &detectedApplications)
			}
			if len(scraped.scripts) > 0 && app.Scripts != nil {
				analyzeScripts(app, scraped.scripts, &detectedApplications)
			}
			if len(scraped.meta) > 0 && app.Meta != nil {
				analyzeMeta(app, scraped.meta, &detectedApplications)
			}
		}(app)
	}

	wg.Wait()

	for _, app := range detectedApplications {
		if app.excludes != nil {
			resolveExcludes(&detectedApplications, app.excludes)
		}
		if app.implies != nil {
			resolveImplies(&wapp.Apps, &detectedApplications, app.implies)
		}
	}
	res := []map[string]interface{}{}
	for _, app := range detectedApplications {
		// log.Printf("URL: %-25s DETECTED APP: %-20s VERSION: %-8s CATEGORIES: %v", url, app.Name, app.Version, app.Categories)
		res = append(res, map[string]interface{}{"name": app.Name, "version": app.Version, "categories": app.Categories})
	}
	if wapp.JSON {
		j, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return string(j), nil
	}
	return res, nil
}

func analyzeURL(app *application, url string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.URL)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil && pattrn.regex.MatchString(url) {
				appMu.Lock()
				if _, ok := (*detectedApplications)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplications)[resApp.Name] = resApp
					detectVersion(resApp, pattrn, &url)
				}
				appMu.Unlock()
			}
		}
	}
}

func analyzeScripts(app *application, scripts []string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.Scripts)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil {
				for _, script := range scripts {
					if pattrn.regex.MatchString(script) {
						appMu.Lock()
						if _, ok := (*detectedApplications)[app.Name]; !ok {
							resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
							(*detectedApplications)[resApp.Name] = resApp
							detectVersion(resApp, pattrn, &script)
						}
						appMu.Unlock()
					}
				}
			}
		}
	}
}

func analyzeHeaders(app *application, headers map[string][]string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.Headers)
	for headerName, v := range patterns {
		headerNameLowerCase := strings.ToLower(headerName)
		for _, pattrn := range v {
			if headersSlice, ok := headers[headerNameLowerCase]; ok {
				for _, header := range headersSlice {
					if pattrn.regex != nil && pattrn.regex.MatchString(header) {
						appMu.Lock()
						if _, ok := (*detectedApplications)[app.Name]; !ok {
							resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
							(*detectedApplications)[resApp.Name] = resApp
							detectVersion(resApp, pattrn, &header)
						}
						appMu.Unlock()
					}
				}
			}
		}
	}
}

func analyzeCookies(app *application, cookies map[string]string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.Cookies)
	for cookieName, v := range patterns {
		cookieNameLowerCase := strings.ToLower(cookieName)
		for _, pattrn := range v {
			if cookie, ok := cookies[cookieNameLowerCase]; ok && pattrn.regex != nil && pattrn.regex.MatchString(cookie) {
				appMu.Lock()
				if _, ok := (*detectedApplications)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplications)[resApp.Name] = resApp
					detectVersion(resApp, pattrn, &cookie)
				}
				appMu.Unlock()
			}
		}
	}
}

func analyzeHTML(app *application, html string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.HTML)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil && pattrn.regex.MatchString(html) {
				appMu.Lock()
				if _, ok := (*detectedApplications)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplications)[resApp.Name] = resApp
					detectVersion(resApp, pattrn, &html)
				}
				appMu.Unlock()
			}
		}

	}
}

func analyzeMeta(app *application, metas map[string][]string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.Meta)
	for metaName, v := range patterns {
		metaNameLowerCase := strings.ToLower(metaName)
		for _, pattrn := range v {
			if metaSlice, ok := metas[metaNameLowerCase]; ok {
				for _, meta := range metaSlice {
					if pattrn.regex != nil && pattrn.regex.MatchString(meta) {
						appMu.Lock()
						if _, ok := (*detectedApplications)[app.Name]; !ok {
							resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
							(*detectedApplications)[resApp.Name] = resApp
							detectVersion(resApp, pattrn, &meta)
						}
						appMu.Unlock()
					}
				}
			}
		}
	}
}

func detectVersion(app *resultApp, pattrn *pattern, value *string) {
	versions := make(map[string]interface{})
	version := pattrn.version
	if slices := pattrn.regex.FindAllStringSubmatch(*value, -1); slices != nil {
		for _, slice := range slices {
			for i, match := range slice {
				reg, _ := regexp.Compile(fmt.Sprintf("%s%d%s", "\\\\", i, "\\?([^:]+):(.*)$"))
				ternary := reg.FindAllString(version, -1)
				if ternary != nil && len(ternary) == 3 {
					version = strings.Replace(version, ternary[0], ternary[1], -1)
				}
				reg2, _ := regexp.Compile(fmt.Sprintf("%s%d", "\\\\", i))
				version = reg2.ReplaceAllString(version, match)
			}
		}
		if _, ok := versions[version]; ok != true && version != "" {
			versions[version] = struct{}{}
		}
		if len(versions) != 0 {
			for ver := range versions {
				if ver > app.Version {
					app.Version = ver
				}
			}
		}
	}
}

type pattern struct {
	str        string
	regex      *regexp.Regexp
	version    string
	confidence string
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
			appPattern := &pattern{}
			slice := strings.Split(str, "\\;")
			for i, item := range slice {
				if item == "" {
					continue
				}
				if i > 0 {
					additional := strings.Split(item, ":")
					if len(additional) > 1 {
						if additional[0] == "version" {
							appPattern.version = additional[1]
						} else {
							appPattern.confidence = additional[1]
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

func parseImpliesExcludes(value interface{}) (array []string) {
	switch item := value.(type) {
	case string:
		array = append(array, item)
	case []string:
		return item
	}
	return array
}

func resolveExcludes(detected *map[string]*resultApp, value interface{}) {
	excludedApps := parseImpliesExcludes(value)
	for _, excluded := range excludedApps {
		delete(*detected, excluded)
	}
}

func resolveImplies(apps *map[string]*application, detected *map[string]*resultApp, value interface{}) {
	impliedApps := parseImpliesExcludes(value)
	for _, implied := range impliedApps {
		app, ok := (*apps)[implied]
		if _, ok2 := (*detected)[implied]; ok && !ok2 {
			resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
			(*detected)[implied] = resApp
			if app.Implies != nil {
				resolveImplies(apps, detected, app.Implies)
			}
		}
	}
}

func parseCategories(app *application, categoriesCatalog *map[string]*category) {
	for _, categoryID := range app.Cats {
		app.Categories = append(app.Categories, (*categoriesCatalog)[strconv.Itoa(categoryID)].Name)
	}
}
