package scraper

type ScrapedURL struct {
	URL    string
	Status int
}

type ScrapedData struct {
	URLs    []ScrapedURL
	HTML    string
	Headers map[string][]string
	Scripts []string
	Cookies map[string]string
	Meta    map[string][]string
	DNS     map[string][]string
}

// Scraper is an interface for different scrapping brower (colly, rod)
type Scraper interface {
	Init() error
	CanRenderPage() bool
	Scrape(paramURL string) (*ScrapedData, error)
	EvalJS(jsProp string) (*string, error)
	SearchDom(domSelector string) (*string, error)
}
