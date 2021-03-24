package scraper

import (
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	log "github.com/sirupsen/logrus"
)

type RodScraper struct {
	Browser               *rod.Browser
	Page                  *rod.Page
	TimeoutSeconds        int
	LoadingTimeoutSeconds int
}

func (s *RodScraper) CanRenderPage() bool {
	return true
}

func (s *RodScraper) Init() error {
	log.Infoln("Rod initialization")
	return rod.Try(func() {
		s.Browser = rod.
			New().
			MustConnect().
			MustIgnoreCertErrors(true)
	})
}

func (s *RodScraper) Scrape(paramURL string) (*ScrapedData, error) {

	scraped := &ScrapedData{}

	var e proto.NetworkResponseReceived
	s.Page = s.Browser.MustPage("")
	wait := s.Page.WaitEvent(&e)
	go s.Page.MustHandleDialog()

	errRod := rod.Try(func() {
		s.Page.
			Timeout(time.Duration(s.TimeoutSeconds) * time.Second).
			MustNavigate(paramURL)
	})
	if errRod != nil {
		log.Errorf("Error while visiting %s : %s", paramURL, errRod.Error())
		return scraped, errRod
	}

	wait()

	scraped.URLs = ScrapedURL{e.Response.URL, e.Response.Status}
	scraped.Headers = make(map[string][]string)
	for header, value := range e.Response.Headers {
		lowerCaseKey := strings.ToLower(header)
		scraped.Headers[lowerCaseKey] = append(scraped.Headers[lowerCaseKey], value.String())
	}

	scraped.DNS = scrapeDNS(paramURL)

	//TODO : headers and cookies could be parsed before load completed
	errRod = rod.Try(func() {
		s.Page.
			Timeout(time.Duration(s.LoadingTimeoutSeconds) * time.Second).
			MustWaitLoad()
	})
	if errRod != nil {
		log.Errorf("Error while loading %s : %s", paramURL, errRod.Error())
		return scraped, errRod
	}

	scraped.HTML = s.Page.MustHTML()

	scripts, _ := s.Page.Elements("script")
	for _, script := range scripts {
		if src, _ := script.Property("src"); src.Val() != nil {
			scraped.Scripts = append(scraped.Scripts, src.String())
		}
	}

	metas, _ := s.Page.Elements("meta")
	scraped.Meta = make(map[string][]string)
	for _, meta := range metas {
		name, _ := meta.Attribute("name")
		if name == nil {
			name, _ = meta.Attribute("property")
		}
		if name != nil {
			if content, _ := meta.Attribute("content"); content != nil {
				nameLower := strings.ToLower(*name)
				scraped.Meta[nameLower] = append(scraped.Meta[nameLower], *content)
			}
		}
	}

	scraped.Cookies = make(map[string]string)
	str := []string{}
	cookies, _ := s.Page.Cookies(str)
	for _, cookie := range cookies {
		scraped.Cookies[cookie.Name] = cookie.Value
	}

	return scraped, nil
}

func (s *RodScraper) EvalJS(jsProp string) (*string, error) {
	res, err := s.Page.Eval(jsProp)
	if err == nil && res != nil && res.Value.Val() != nil {
		value := ""
		if res.Type == "string" || res.Type == "number" {
			value = res.Value.String()
		}
		return &value, err
	} else {
		return nil, err
	}
}
