package gowap

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/extensions"
)

// CustomError type to have more information about error
type wappalyzerError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (e *wappalyzerError) Error() string {
	return fmt.Sprintf("Error code: %d, msg: %s", e.Code, e.Msg)
}

type collyData struct {
	html    string
	headers map[string][]string
	scripts []string
	cookies map[string]string
}

type temp struct {
	Apps       map[string]*json.RawMessage `json:"apps"`
	Categories map[string]*json.RawMessage `json:"categories"`
}
type application struct {
	Name     string
	Cats     []int                  `json:"cats,omitempty"`
	Cookies  interface{}            `json:"cookies,omitempty"`
	Js       interface{}            `json:"js,omitempty"`
	Headers  interface{}            `json:"headers,omitempty"`
	HTML     interface{}            `json:"html,omitempty"`
	Excludes interface{}            `json:"excludes,omitempty"`
	Implies  interface{}            `json:"implies,omitempty"`
	Meta     map[string]interface{} `json:"meta,omitempty"`
	Scripts  interface{}            `json:"script,omitempty"`
	URL      string                 `json:"url,omitempty"`
	Website  string                 `json:"website,omitempty"`
	Detected bool
	Version  string
}

type category struct {
	ReferenceNumber string
	Name            string `json:"name,omitempty"`
	Priority        int    `json:"priority,omitempty"`
}

// Wappalyzer implements analyze method as original wappalyzer does
type Wappalyzer struct {
	Collector       *colly.Collector
	Apps            map[string]*application
	Categories      []*category
	RandomUserAgent bool
	MaxDepth        int
}

// Init initializes wappalyzer
func Init(signaturesFilePath string, randomUserAgent bool, MaxDepth int) (wapp *Wappalyzer, err error) {
	wapp = &Wappalyzer{}
	wapp.Collector = colly.NewCollector(
		colly.IgnoreRobotsTxt(),
		colly.MaxDepth(MaxDepth),
		// colly.Debugger(&debug.LogDebugger{}),
	)
	extensions.Referrer(wapp.Collector)
	if randomUserAgent {
		extensions.RandomUserAgent(wapp.Collector)
	}

	appsFile, err := ioutil.ReadFile(signaturesFilePath)
	if err != nil {
		fmt.Printf("[!] Couldn't open file at %s\n", signaturesFilePath)
		return nil, err
	}
	temporary := &temp{}
	err = json.Unmarshal(appsFile, &temporary)
	if err != nil {
		fmt.Printf("[!] Couldn't unmarshal apps.json file: %s\n", err)
	}
	wapp.Apps = make(map[string]*application)
	for k, v := range temporary.Apps {
		app := &application{}
		app.Name = k
		if err = json.Unmarshal(*v, app); err != nil {
			return nil, err
		}
		wapp.Apps[k] = app
	}
	for k, v := range temporary.Categories {
		catg := &category{}
		catg.ReferenceNumber = k
		if err = json.Unmarshal(*v, catg); err != nil {
			return nil, err
		}
		wapp.Categories = append(wapp.Categories, catg)
	}
	return wapp, nil
}

// Analyze retrieves application stack used on the provided web-site
func (wapp *Wappalyzer) Analyze(url string) (result interface{}, err error) {
	// var detectedApplications map[string]*application
	scraped := &collyData{}

	wapp.Collector.OnRequest(func(r *colly.Request) {
		log.Infof("Visiting %s", r.URL)
	})

	wapp.Collector.OnError(func(_ *colly.Response, err error) {
		log.Error(err)
	})

	wapp.Collector.OnResponse(func(r *colly.Response) {
		log.Infof("Visited %s", r.Request.URL)
		log.Infof("Status code: %d", r.StatusCode)
		if r.StatusCode != 200 {
			err = fmt.Errorf("status code is %d", r.StatusCode)
			return
		}

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

	wapp.Collector.OnHTML("script", func(e *colly.HTMLElement) {
		scraped.scripts = append(scraped.scripts, e.Attr("src"))
		fmt.Println(e.Attr("src"))
	})

	err = wapp.Collector.Visit(url)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	for _, app := range wapp.Apps {
		// var patterns map[string][]*pattern
		analyzeURL(app, url)

		// if app.HTML != nil {
		// 	patterns := parsePatterns(app.HTML)
		// }
		// if app.Headers != nil {
		// 	patterns := parsePatterns(app.Headers)
		// }
		// if app.Cookies != nil {
		// 	patterns := parsePatterns(app.Cookies)
		// }
		// if app.Scripts != nil {
		// 	patterns := parsePatterns(app.Scripts)
		// }
		if app.Detected {
			fmt.Println(app.Name)
		}
	}
	return scraped, nil
}

func analyzeURL(app *application, url string) {
	patterns := parsePatterns(app.URL)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil && pattrn.regex.Match([]byte(url)) {
				app.Detected = true
				fmt.Println("MATCHED!!!")
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
			parsed[k] = append(parsed[k], v.(string))
		}
	case []interface{}:
		var slice []string
		for _, v := range ptrn {
			slice = append(slice, v.(string))
		}
		parsed["main"] = slice
	default:
		fmt.Printf("[!] Unkown type in parsePatterns: %T\n", ptrn)
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
					reg, err := regexp.Compile(fmt.Sprintf("%s%s", "(?i)", strings.Replace(item, "/", `\/`, -1)))
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

type detectedApp struct {
	Name       string   `json:"name,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
	Version    string   `json:"version,omitempty"`
	Website    string   `json:"website,omitempty"`
	Categories []string `json:"categories,omitempty"`
}
type result struct {
	Urls         []string       `json:"urls,omitempty"`
	Applications []*detectedApp `json:"applications,omitempty"`
}
