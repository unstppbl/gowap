package scraper

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	log "github.com/sirupsen/logrus"
)

type RodScraper struct {
	Browser                *rod.Browser
	Page                   *rod.Page
	BrowserTimeoutSeconds  int
	NetworkTimeoutSeconds  int
	PageLoadTimeoutSeconds int
}

func (s *RodScraper) CanRenderPage() bool {
	return true
}

func (s *RodScraper) Init() error {
	errRod := rod.Try(func() {
		s.Browser = rod.
			New().
			Timeout(time.Duration(s.BrowserTimeoutSeconds) * time.Second).
			MustConnect()
	})
	if errors.Is(errRod, context.DeadlineExceeded) {
		log.Errorf("Timeout reached (%ds) while connecting Browser", s.BrowserTimeoutSeconds)
		return errRod
	} else if errRod != nil {
		log.Errorf("Error while connecting Browser")
		return errRod
	}

	s.Browser.IgnoreCertErrors(true)
	return nil
}

func (s *RodScraper) Scrape(paramURL string) (*ScrapedData, error) {

	scraped := &ScrapedData{}

	var e proto.NetworkResponseReceived
	s.Page = s.Browser.MustPage("")
	wait := s.Page.WaitEvent(&e)
	go s.Page.MustHandleDialog()

	errRod := rod.Try(func() {
		s.Page.
			Timeout(time.Duration(s.NetworkTimeoutSeconds) * time.Second).
			MustNavigate(paramURL)
	})
	if errors.Is(errRod, context.DeadlineExceeded) {
		log.Errorf("Timeout reached (%ds) while visiting %s", s.NetworkTimeoutSeconds, paramURL)
		return scraped, errRod
	} else if errRod != nil {
		log.Errorf("Error while visiting %s", paramURL)
		return scraped, errRod
	}

	wait()

	scraped.URLs = append(scraped.URLs, ScrapedURL{e.Response.URL, e.Response.Status})
	scraped.Headers = make(map[string][]string)
	for header, value := range e.Response.Headers {
		lowerCaseKey := strings.ToLower(header)
		scraped.Headers[lowerCaseKey] = append(scraped.Headers[lowerCaseKey], value.String())
	}

	u, _ := url.Parse(paramURL)
	parts := strings.Split(u.Hostname(), ".")
	domain := parts[len(parts)-2] + "." + parts[len(parts)-1]
	scraped.DNS = make(map[string][]string)
	nsSlice, _ := net.LookupNS(domain)
	for _, ns := range nsSlice {
		scraped.DNS["NS"] = append(scraped.DNS["NS"], string(ns.Host))
	}
	mxSlice, _ := net.LookupMX(domain)
	for _, mx := range mxSlice {
		scraped.DNS["MX"] = append(scraped.DNS["MX"], string(mx.Host))
	}
	txtSlice, _ := net.LookupTXT(domain)
	for _, txt := range txtSlice {
		scraped.DNS["TXT"] = append(scraped.DNS["TXT"], txt)
	}
	cname, _ := net.LookupCNAME(domain)
	scraped.DNS["CNAME"] = append(scraped.DNS["CNAME"], cname)

	//TODO : headers and cookies could be parsed before load completed
	errRod = rod.Try(func() {
		s.Page.
			Timeout(time.Duration(s.PageLoadTimeoutSeconds) * time.Second).
			MustWaitLoad()
	})
	if errors.Is(errRod, context.DeadlineExceeded) {
		log.Errorf("Timeout reached (%ds) while loading %s", s.PageLoadTimeoutSeconds, paramURL)
		return scraped, errRod
	} else if errRod != nil {
		log.Errorf("Error while visiting %s", paramURL)
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
		name, _ := meta.Property("name")
		if name.Val() == nil {
			name, _ = meta.Property("property")
		}
		if name.Val() != nil {
			if content, _ := meta.Property("content"); content.Val() != nil {
				nameLower := strings.ToLower(name.String())
				scraped.Meta[nameLower] = append(scraped.Meta[nameLower], content.String())
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

func (s *RodScraper) SearchDom(domSelector string) (*string, error) {
	return nil, nil
}
