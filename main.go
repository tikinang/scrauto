package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gocolly/colly"
)

type Car struct {
	Link     string `json:"link"`
	Info     string `json:"info"`
	Price    uint64 `json:"price"`
	Driven   uint64 `json:"driven"`
	Power    uint64 `json:"power"`
	Year     uint64 `json:"year"`
	Bodywork string `json:"bodywork"`
	Seller   string `json:"seller"`
	Locality string `json:"locality"`
}

func (r *Car) Csv() []string {
	return []string{
		r.Link,
		r.Info,
		strconv.FormatUint(r.Price, 10),
		strconv.FormatUint(r.Driven, 10),
		strconv.FormatUint(r.Power, 10),
		strconv.FormatUint(r.Year, 10),
		r.Bodywork,
		r.Seller,
		r.Locality,
	}
}

var blueprint, cacheFile string
var exportCsv bool
var cache map[string]*Car // map[Car.Link]Car

func main() {
	var stringArgs string
	flag.StringVar(
		&blueprint,
		"blueprint",
		"https://www.sauto.cz/inzerce/osobni/mazda/3?cena-do=400000&vyrobeno-od=2014&palivo=benzin&vybava=bluetooth&typ=%s",
		"sauto url format",
	)
	flag.StringVar(
		&cacheFile,
		"cache",
		"sauto.json",
		"cache json filepath",
	)
	flag.StringVar(
		&stringArgs,
		"args",
		"sedanlimuzina|hatchback",
		"pipe separated list of args to iterate over in blueprint",
	)
	flag.BoolVar(
		&exportCsv,
		"csv",
		false,
		"export to csv file",
	)
	flag.Parse()

	if err := loadCache(); err != nil {
		panic(err)
	}
	if exportCsv {
		fmt.Println("exporting to csv...")
		exportToCsv()
		return
	}

	defer saveCache()
	for _, arg := range strings.Split(stringArgs, "|") {
		if err := visit(arg); err != nil {
			fmt.Println(err)
		}
	}
}

func exportToCsv() {
	f, err := os.Create(fmt.Sprintf("%s.csv", cacheFile))
	if err != nil {
		panic(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{
		"link",
		"nadpis",
		"cena (tis. kč)",
		"najeto (tis. km)",
		"výkon (kW)",
		"rok výroby",
		"karoserie",
		"prodejce",
		"lokalita",
	}); err != nil {
		panic(err)
	}
	for _, car := range cache {
		if err := w.Write(car.Csv()); err != nil {
			panic(err)
		}
	}
}

func loadCache() error {
	cache = make(map[string]*Car)
	f, err := os.Open(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(&cache)
}

func saveCache() {
	f, err := os.Create(cacheFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "\t")
	err = enc.Encode(cache)
	if err != nil {
		fmt.Println(err)
		return
	}
}

func visit(bodywork string) error {
	c := colly.NewCollector()
	c.OnHTML(".c-item-list__list", func(e *colly.HTMLElement) {
		e.ForEach(".c-item", func(_ int, e *colly.HTMLElement) {
			link := e.ChildAttr(".c-item__link", "href")
			if e.DOM.Parent().HasClass("c-preferred-list__list") {
				fmt.Printf("skip ad [%s]\n", link)
				return
			}
			if _, has := cache[link]; has {
				fmt.Printf("updating car [%s]\n", link)
			} else {
				fmt.Printf("new car [%s]\n", link)
			}
			infos := strings.Split(e.ChildText(".c-item__info"), ", ")
			cache[link] = &Car{
				Link:     link,
				Info:     e.ChildText(".c-item__name"),
				Price:    getThousands(e.ChildText(".c-item__price")),
				Driven:   getThousands(infos[1]),
				Year:     getThousands(infos[0]),
				Seller:   e.ChildText(".c-item__seller"),
				Locality: e.ChildText(".c-item__locality"),
			}
			if err := e.Request.Visit(link); err != nil {
				fmt.Println(err)
			}
		})
	})
	c.OnHTML(".c-car-properties", func(e *colly.HTMLElement) {
		e.ForEach("li", func(_ int, e *colly.HTMLElement) {
			if e.ChildText(".c-car-properties__tile-label") == "Výkon" {
				link := e.Request.URL.String()
				cache[link].Power, _ = strconv.ParseUint(strings.TrimSuffix(e.ChildText(".c-car-properties__tile-value"), " kW"), 10, 64)
			}
			if e.ChildText(".c-car-properties__tile-label") == "Karoserie" {
				link := e.Request.URL.String()
				cache[link].Bodywork = e.ChildText(".c-car-properties__tile-value")
			}
		})
	})
	c.OnHTML(".c-paging__btn-next", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		fmt.Printf("next page [%s]\n", link)
		if err := e.Request.Visit(link); err != nil {
			fmt.Println(err)
		}
	})
	link := fmt.Sprintf(blueprint, bodywork)
	fmt.Printf("visit [%s]\n", link)
	return c.Visit(link)
}

var reg = regexp.MustCompile(`-?\d[\d,]*[.]?[\d{2}]*`)

func getThousands(in string) uint64 {
	i, err := strconv.ParseUint(reg.FindString(in), 10, 64)
	if err != nil {
		panic(err)
	}
	return i
}
