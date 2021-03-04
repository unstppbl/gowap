package gowap

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
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

type scrapedData struct {
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
	Browser                *rod.Browser
	BrowserTimeoutSeconds  int
	NetworkTimeoutSeconds  int
	PageLoadTimeoutSeconds int
	Apps                   map[string]*application
	Categories             map[string]*category
	JSON                   bool
}

// Init initializes wappalyzer
func Init(appsJSONPath string, JSON bool) (wapp *Wappalyzer, err error) {
	wapp = &Wappalyzer{BrowserTimeoutSeconds: 4, NetworkTimeoutSeconds: 3, PageLoadTimeoutSeconds: 2}

	errRod := rod.Try(func() {
		wapp.Browser = rod.New().Timeout(time.Duration(wapp.BrowserTimeoutSeconds) * time.Second).MustConnect()
	})
	if errors.Is(errRod, context.DeadlineExceeded) {
		log.Errorf("Timeout reached (%ds) while connecting Browser", wapp.BrowserTimeoutSeconds)
		return wapp, errRod
	} else if errRod != nil {
		log.Errorf("Error while connecting Browser")
		return wapp, errRod
	}

	wapp.Browser.IgnoreCertErrors(true)

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
	Confidence int      `json:"confidence"`
	excludes   interface{}
	implies    interface{}
}

type detected struct {
	Mu   *sync.Mutex
	Apps map[string]*resultApp
}

// Analyze retrieves application stack used on the provided web-site
func (wapp *Wappalyzer) Analyze(url string) (result interface{}, err error) {

	detectedApplications := &detected{new(sync.Mutex), make(map[string]*resultApp)}
	scraped := &scrapedData{}
	res := []map[string]interface{}{}

	var e proto.NetworkResponseReceived
	page := wapp.Browser.MustPage("")
	wait := page.WaitEvent(&e)
	go page.MustHandleDialog()

	errRod := rod.Try(func() {
		page.Timeout(time.Duration(wapp.NetworkTimeoutSeconds) * time.Second).MustNavigate(url)
	})
	if errors.Is(errRod, context.DeadlineExceeded) {
		log.Errorf("Timeout reached (%ds) while visiting %s", wapp.NetworkTimeoutSeconds, url)
		return res, errRod
	} else if errRod != nil {
		log.Errorf("Error while visiting %s", url)
		return res, errRod
	}

	wait()

	scraped.status = e.Response.Status
	scraped.url = e.Response.URL
	scraped.headers = make(map[string][]string)
	for header, value := range e.Response.Headers {
		lowerCaseKey := strings.ToLower(header)
		scraped.headers[lowerCaseKey] = append(scraped.headers[lowerCaseKey], value.String())
	}

	//TODO : headers and cookies could be parsed before load completed
	errRod = rod.Try(func() {
		page.Timeout(time.Duration(wapp.PageLoadTimeoutSeconds) * time.Second).MustWaitLoad()
	})
	if errors.Is(errRod, context.DeadlineExceeded) {
		log.Errorf("Timeout reached (%ds) while loading %s", wapp.PageLoadTimeoutSeconds, url)
		return res, errRod
	} else if errRod != nil {
		log.Errorf("Error while visiting %s", url)
		return res, errRod
	}

	scraped.html = page.MustHTML()

	scripts, _ := page.Elements("script")
	for _, script := range scripts {
		if src, _ := script.Property("src"); src.Val() != nil {
			scraped.scripts = append(scraped.scripts, src.String())
		}
	}

	metas, _ := page.Elements("meta")
	scraped.meta = make(map[string][]string)
	for _, meta := range metas {
		name, _ := meta.Property("name")
		if name.Val() == nil {
			name, _ = meta.Property("property")
		}
		if name.Val() != nil {
			if content, _ := meta.Property("content"); content.Val() != nil {
				nameLower := strings.ToLower(name.String())
				scraped.meta[nameLower] = append(scraped.meta[nameLower], content.String())
			}
		}
	}

	scraped.cookies = make(map[string]string)
	str := []string{}
	cookies, _ := page.Cookies(str)
	for _, cookie := range cookies {
		scraped.cookies[cookie.Name] = cookie.Value
	}

	for _, app := range wapp.Apps {
		wg.Add(1)
		go func(app *application) {
			defer wg.Done()
			analyzeURL(app, url, detectedApplications)
			if app.Js != nil {
				analyseJS(app, page, detectedApplications)
			}
			if app.HTML != nil {
				analyzeHTML(app, scraped.html, detectedApplications)
			}
			if len(scraped.headers) > 0 && app.Headers != nil {
				analyzeHeaders(app, scraped.headers, detectedApplications)
			}
			if len(scraped.cookies) > 0 && app.Cookies != nil {
				analyzeCookies(app, scraped.cookies, detectedApplications)
			}
			if len(scraped.scripts) > 0 && app.Scripts != nil {
				analyzeScripts(app, scraped.scripts, detectedApplications)
			}
			if len(scraped.meta) > 0 && app.Meta != nil {
				analyzeMeta(app, scraped.meta, detectedApplications)
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

	for _, app := range detectedApplications.Apps {
		// log.Printf("URL: %-25s DETECTED APP: %-20s VERSION: %-8s CATEGORIES: %v", url, app.Name, app.Version, app.Categories)
		res = append(res, map[string]interface{}{"name": app.Name, "confidence": app.Confidence, "version": app.Version, "categories": app.Categories})
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

func analyzeURL(app *application, url string, detectedApplications *detected) {
	patterns := parsePatterns(app.URL)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil && pattrn.regex.MatchString(url) {
				version := detectVersion(pattrn, &url)
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

// analyseJS evals the JS properties and tries to match
func analyseJS(app *application, page *rod.Page, detectedApplications *detected) {
	patterns := parsePatterns(app.Js)
	for jsProp, v := range patterns {
		res, err := page.Eval(jsProp)
		if err == nil && res != nil && res.Value.Val() != nil {
			value := ""
			if res.Type == "string" || res.Type == "number" {
				value = res.Value.String()
			}
			for _, pattrn := range v {
				if pattrn.str == "" || (pattrn.regex != nil && pattrn.regex.MatchString(value)) {
					version := detectVersion(pattrn, &value)
					addApp(app, detectedApplications, version, pattrn.confidence)
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
		resApp := &resultApp{app.Name, version, app.Categories, confidence, app.Excludes, app.Implies}
		(*detectedApplications).Apps[resApp.Name] = resApp
	} else {
		if (*detectedApplications).Apps[app.Name].Version == "" {
			(*detectedApplications).Apps[app.Name].Version = version
		}
		if confidence > (*detectedApplications).Apps[app.Name].Confidence {
			(*detectedApplications).Apps[app.Name].Confidence = confidence
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
					additional := strings.Split(item, ":")
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
				resApp := &resultApp{app.Name, implied.version, app.Categories, implied.confidence, app.Excludes, app.Implies}
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
