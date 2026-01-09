package main

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var host string = "https://www.sjsu.edu/"

var disabledLinks = []string{}

var client = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after 5 redirects")
		}
		return nil
	},
	Timeout: 10 * time.Second,
}

type Element struct {
}

type Queue struct {
	totalQueued int
	number      int
	elements    []string
	mu          sync.Mutex
}

type CrawledSet struct {
	data   map[uint64]bool
	number int
	mu     sync.Mutex
}

func (q *Queue) enqueue(url string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.elements = append(q.elements, url)
	q.totalQueued++
	q.number++
}

func (q *Queue) dequeue() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	url := q.elements[0]
	q.elements = q.elements[1:]
	q.number--
	return url
}

func (q *Queue) size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.number
}

func (c *CrawledSet) add(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[hashUrl(url)] = true
	c.number++
}

func hashUrl(url string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(url))
	return h.Sum64()
}

func (c *CrawledSet) contains(url string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data[hashUrl(url)]
}

func (c *CrawledSet) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.number
}

func robotsTxtFetcher() {
	client := &http.Client{
		Timeout: 8 * time.Second,
	}
	req, err := http.NewRequest("GET", "https://sjsu.edu/robots.txt", nil)
	if err != nil {
		log.Printf("Error making GET request: %s", err)
		return
	}
	req.Header.Set("User-Agent", "Golang_Custom_Bot/1.0")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error making GET request: %s", err)
		return
	}
	defer resp.Body.Close()
	robotsTxt, err := io.ReadAll(resp.Body)
	stringReader := strings.NewReader(string(robotsTxt))
	scanner := bufio.NewScanner(stringReader)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Disallow:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				disabledLinks = append(disabledLinks, fields[1])
			}
		}
	}
}

func isAllowed(url string) bool {
	for _, disallowed := range disabledLinks {
		if strings.Contains(url, disallowed) {
			return false
		}
	}
	return true
}

func printError(err error, message string) {
	log.Println(message, err)
}

var fileMutex sync.Mutex

func writeToFile(title string, currUrl string) {
	fileMutex.Lock()
	defer fileMutex.Unlock()
	file, err := os.OpenFile("result.txt", os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		printError(err, "Error at line 192\t")
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "{\n\ttitle: %s,\n\turl: %s\n}", title, currUrl)
	if err != nil {
		printError(err, "Error at line 198\t")
	}
}

func fetcher(url string, c chan *goquery.Document) {
	for i := range 5 {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			printError(err, "Error at line 112")
			continue
		}
		req.Header.Set("User-Agent", "Golang_Custom_Bot/1.0")
		if i > 0 {
			time.Sleep(time.Duration(2*i) * time.Second)
		}
		resp, err := client.Do(req)
		if err != nil {
			printError(err, "Error at line 174")
			continue
		}
		if resp.StatusCode == http.StatusOK {
			doc, err := goquery.NewDocumentFromReader(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Fatal(err)
			}
			c <- doc
			return
		} else {
			log.Printf("Error making GET request: %s", resp.Status)
			resp.Body.Close()
		}
	}
	c <- nil
}

func parser(doc *goquery.Document, queue *Queue, currUrl string, crawledSet *CrawledSet) {

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
				return
			}

			if strings.HasPrefix(href, "/") {
				href = strings.TrimSuffix(host, "/") + href
			} else if !strings.HasPrefix(href, "http") {
				return
			}

			if strings.HasPrefix(href, host) && isAllowed(href) && !crawledSet.contains(href) {
				queue.enqueue(href)
			}
		}
	})

	title := doc.Find("title").Text()

	writeToFile(title, currUrl)

}

var wg sync.WaitGroup

func main() {
	queue := Queue{totalQueued: 0, number: 0, elements: make([]string, 0)}
	robotsTxtFetcher()
	crawled := CrawledSet{data: make(map[uint64]bool)}

	crawled.add(host)
	c := make(chan *goquery.Document)
	go fetcher(host, c)
	content := <-c
	if content != nil {
		parser(content, &queue, host, &crawled)
	}

	ticker := time.NewTicker(1 * time.Second)
	done := make(chan bool)
	crawlerStats := CrawlerStats{pagesPerMinute: "0 0\n", crawledRatioPerMinute: "0 0\n", startTime: time.Now()}

	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				crawlerStats.update(&crawled, &queue, t)
			}
		}
	}()

	for queue.size() > 0 && crawled.size() < 500 {
		url := queue.dequeue()
		crawled.add(url)
		fetchChan := make(chan *goquery.Document)
		go fetcher(url, fetchChan)
		content := <-fetchChan
		if content != nil {
			wg.Add(1)
			go func(doc *goquery.Document, url string) {
				defer wg.Done()
				parser(doc, &queue, url, &crawled)
			}(content, url)
		}
	}

	wg.Wait()

	ticker.Stop()
	done <- true

	fmt.Println("\n------------------CRAWLER STATS------------------")
	fmt.Printf("Total queued: %d\n", queue.totalQueued)
	fmt.Printf("To be crawled (Queue) size: %d\n", queue.size())
	fmt.Printf("Crawled size: %d\n", crawled.size())
	crawlerStats.print()
}

type CrawlerStats struct {
	startTime             time.Time
	pagesPerMinute        string // 0 0 \n 1 100
	crawledRatioPerMinute string
}

func (c *CrawlerStats) update(crawled *CrawledSet, queue *Queue, t time.Time) {
	c.pagesPerMinute += fmt.Sprintf("%f %d\n", t.Sub(c.startTime).Minutes(), crawled.size())
	c.crawledRatioPerMinute += fmt.Sprintf("%f %f\n", t.Sub(c.startTime).Minutes(), float64(crawled.size())/float64(queue.size()))
}

func (c *CrawlerStats) print() {
	fmt.Println("Pages crawled per minute:")
	fmt.Println(c.pagesPerMinute)
	fmt.Println("Crawl to Queued Ratio per minute:")
	fmt.Println(c.crawledRatioPerMinute)
}
