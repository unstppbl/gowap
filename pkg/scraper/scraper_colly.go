package scraper

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gocolly/colly"
	extensions "github.com/gocolly/colly/extensions"

	log "github.com/sirupsen/logrus"
)

func (s *CollyScraper) CanRenderPage() bool {
	return false
}

type CollyScraper struct {
	Collector             *colly.Collector
	Transport             *http.Transport
	TimeoutSeconds        int
	LoadingTimeoutSeconds int
	UserAgent             string
}

func (s *CollyScraper) Init() error {
	log.Infoln("Colly initialization")
	s.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: time.Second * time.Duration(s.TimeoutSeconds),
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ExpectContinueTimeout: time.Duration(s.TimeoutSeconds) * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	}

	s.Collector = colly.NewCollector()
	s.Collector.IgnoreRobotsTxt = false
	s.Collector.UserAgent = s.UserAgent
	s.Collector.WithTransport(s.Transport)

	extensions.Referer(s.Collector)
	extensions.RandomUserAgent(s.Collector)

	return nil
}

func (s *CollyScraper) Scrape(paramURL string) (*ScrapedData, error) {

	scraped := &ScrapedData{}
	scraped.DNS = scrapeDNS(paramURL)

	s.Collector.OnResponse(func(r *colly.Response) {
		// log.Infof("Visited %s", r.Request.URL)
		scraped.URLs = ScrapedURL{r.Request.URL.String(), r.StatusCode}
		scraped.Headers = make(map[string][]string)
		for k, v := range *r.Headers {
			lowerCaseKey := strings.ToLower(k)
			scraped.Headers[lowerCaseKey] = v
		}

		scraped.HTML = string(r.Body)

		scraped.Cookies = make(map[string]string)
		for _, cookie := range scraped.Headers["set-cookie"] {
			keyValues := strings.Split(cookie, ";")
			for _, keyValueString := range keyValues {
				keyValueSlice := strings.Split(keyValueString, "=")
				if len(keyValueSlice) > 1 {
					key, value := keyValueSlice[0], keyValueSlice[1]
					scraped.Cookies[key] = value
				}
			}
		}
	})

	s.Collector.OnHTML("script", func(e *colly.HTMLElement) {
		scraped.Scripts = append(scraped.Scripts, e.Attr("src"))
	})

	err := s.Collector.Visit(paramURL)

	return scraped, err
}

// Colly cannot eval JS
func (s *CollyScraper) EvalJS(jsProp string) (*string, error) {
	return nil, errors.New("NotImplemented")
}
