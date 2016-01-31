// Reddit audiences crawler
// Rémy Mathieu © 2016
package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	REDDIT_SUBREDDIT_URL = "https://reddit.com/r/"
)

func StartCrawlingJob(a *App) {
	log.Println("info: starts tracking job.")
	ticker := time.NewTicker(time.Second * 30)
	for range ticker.C {
		log.Println("info: tracking job is running.")
		Crawl(a)
	}
	ticker.Stop()
}

// Crawl retrieves the audience of subreddits for which
// the last crawl time is more than some minutes.
func Crawl(a *App) {
	// crawl each subreddit each 5 minutes
	five := time.Minute * 5
	t := time.Now().Add(-five)
	subreddits, err := a.DB().FindSubredditsToCrawl(t)

	if err != nil {
		log.Printf("err: can't retrieve subreddits to crawl: %s\n", err.Error())
	}

	for _, subreddit := range subreddits {
		log.Println("info: crawling", subreddit)
		go func(subreddit string) {
			if audience, err := GetAudience(subreddit); err == nil {
				// store the value and update the last crawl time
				if err := a.DB().InsertSubredditValue(subreddit, audience); err != nil {
					log.Println("err:", err.Error())
				} else {
					log.Printf("info: subreddit %s has %d active users\n", subreddit, audience)
				}
			} else if err != nil {
				log.Println("err:", err.Error())
			}
		}(subreddit)
	}
}

// GetAudience gets the subreddit page on reddit
// and gets the current audience of this subreddit in the DOM.
func GetAudience(subreddit string) (int, error) {
	var audience int
	var err error

	doc, err := goquery.NewDocument(REDDIT_SUBREDDIT_URL + subreddit)
	if err != nil {
		return 0, err
	}

	s := doc.Find("p.users-online span.number").First()

	// it looks like we found a value in the dom
	value := s.Text()
	if len(value) == 0 {
		return 0, fmt.Errorf("can't retrieve subreddit %s audience", subreddit)
	}

	// sometimes it starts with ~
	if strings.HasPrefix(value, "~") {
		value = value[1:]
	}
	// , for thousands etc.
	value = strings.Replace(value, ",", "", -1)
	// finally trim
	value = strings.Trim(value, " ")

	audience, err = strconv.Atoi(value)

	return audience, err
}
