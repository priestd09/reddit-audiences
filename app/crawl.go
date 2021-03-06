// Reddit audiences crawler
// Rémy Mathieu © 2016

package app

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/remeh/reddit-audiences/db"

	"github.com/PuerkitoBio/goquery"
)

const (
	REDDIT_SUBREDDIT_URL           = "https://www.reddit.com/r/"
	MIN_SECONDS_BETWEEN_EACH_CRAWL = 1
)

var subredditsToCrawl chan string

func StartCrawlingJob(a *App) {
	subredditsToCrawl = make(chan string)

	// starts the worker
	go Worker(a)

	// starts the main loop
	// regularly feeding the worker.
	if a.Config.Crawl {
		log.Println("info: starts crawling job.")
		ticker := time.NewTicker(time.Second * 30)
		for range ticker.C {
			log.Println("info: crawling job is running.")
			Feeder(a)
		}
		ticker.Stop()
	}
}

// Worker is the routine dealing with the HTTP
// call + reading hte DOM.
func Worker(a *App) {
	for subreddit := range subredditsToCrawl {
		if audience, subscribers, articles, err := readDOMData(subreddit); err == nil {
			// store the audience+subscribers and update the last crawl time
			// ----------------------
			if err := a.DB().InsertAudienceValue(subreddit, audience, subscribers); err != nil {
				log.Println("err:", err.Error())
			} else {
				log.Printf("info: subreddit %s has %d active users (%d subscribers)\n", subreddit, audience, subscribers)
			}

			// store each articles if not already present at this rank
			// ----------------------
			if err := storeArticles(a, articles); err != nil {
				log.Printf("err: %s", err.Error())
			}
		} else if err != nil {
			log.Println("err:", err.Error())
		}

		r := time.Duration(((rand.Int() % 2) + MIN_SECONDS_BETWEEN_EACH_CRAWL)) * time.Second

		time.Sleep(r) // wait a bit before the next crawl
	}
}

// Feeder retrieves the audience of subreddits for which
// the last crawl time is more than some minutes.
func Feeder(a *App) {
	// crawl each subreddit each 5 minutes
	five := time.Minute * 5
	t := time.Now().Add(-five)
	subreddits, err := a.DB().FindSubredditsToCrawl(t)

	if err != nil {
		log.Printf("err: can't retrieve subreddits to crawl: %s\n", err.Error())
	}

	for _, subreddit := range subreddits {
		log.Println("info: crawling", subreddit)
		subredditsToCrawl <- subreddit
	}
}

// readDOMData gets the subreddit page on reddit
// and gets the current audience, the subscribers and
// the article infos from the DOM.
// NOTE(remy): we stop as soon as we have a DOM error because
// it has great chances that the full DOM is corrupted/not retrieved.
func readDOMData(subreddit string) (int64, int64, []db.Article, error) {
	var audience int64
	var subscribers int64
	var err error

	doc, err := getSubredditPage(REDDIT_SUBREDDIT_URL + subreddit)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("while crawling %s: %s", subreddit, err.Error())
	}

	// audience
	// ----------------------

	s := doc.Find("p.users-online span.number").First()

	value := s.Text()
	if len(value) == 0 {
		return 0, 0, nil, fmt.Errorf("can't retrieve subreddit %s audience: no text value in the dom node.", subreddit)
	}

	if audience, err = cleanInt(value); err != nil {
		return 0, 0, nil, err
	}

	// subscribers
	// ----------------------

	s = doc.Find("span.subscribers span.number").First()

	value = s.Text()
	if len(value) == 0 {
		return 0, 0, nil, fmt.Errorf("can't retrieve subreddit %s subscribers: no text value in the dom node.", subreddit)
	}

	if subscribers, err = cleanInt(value); err != nil {
		return 0, 0, nil, err
	}

	// articles
	// ----------------------

	now := time.Now()
	articles := make([]db.Article, 0)
	s = doc.Find(".link").Each(func(i int, selec *goquery.Selection) {
		l := selec.Find("p.title a.title")
		title := l.First()
		// NOTE(remy): for security reason, we could manually recreate the link
		link, _ := selec.Find("a.comments").Attr("href")
		externalLink, _ := l.Attr("href")
		articleId, _ := selec.Attr("data-fullname")
		author, _ := selec.Attr("data-author")

		// remove the t[1-3]_ from the article id
		for i := 0; i < 4; i++ {
			articleId = strings.TrimPrefix(articleId, fmt.Sprintf("t%d_", i))
		}

		// rank
		strPos := selec.ChildrenFiltered(".rank").First()
		rank, err := strconv.Atoi(strPos.Text())
		if err != nil {
			rank = 0 // it's probably a promoted or stickied article
		}

		// score
		strScore := selec.Find(".score.unvoted").First()
		score, err := strconv.Atoi(strScore.Text())
		if err != nil {
			score = 0
		}

		// comments  count
		strComments := selec.Find("a.comments").First().Text()
		strComments = strings.Trim(strings.Replace(strComments, "comments", "", -1), " ")
		comments, err := strconv.Atoi(strComments)
		if err != nil {
			comments = 0
		}

		promoted := false
		// NOTE(remy): it shouldn't happen because the promoted articles
		// look to be added by javascript.
		if selec.HasClass("promoted") {
			promoted = true
		}

		sticky := false
		if selec.HasClass("stickied") {
			sticky = true
		}

		articles = append(articles, db.Article{
			Subreddit:           subreddit,
			ArticleId:           articleId,
			ArticleTitle:        title.Text(),
			ArticleLink:         link,
			ArticleExternalLink: externalLink,
			Score:               score,
			Comments:            comments,
			Author:              author,
			Rank:                rank,
			CrawlTime:           now,
			Promoted:            promoted,
			Sticky:              sticky,
		})
	})

	return audience, subscribers, articles, err
}

// storeArticles stores all given articles.
func storeArticles(a *App, articles []db.Article) error {
	if len(articles) == 0 {
		return nil
	}

	for _, article := range articles {
		if _, err := a.DB().InsertArticle(article); err != nil {
			return err
		}
	}

	return nil
}

func getSubredditPage(url string) (*goquery.Document, error) {
	r, err := NewRequest(url)
	if err != nil {
		return nil, err
	}

	client := http.Client{
		Timeout: time.Second * 10,
	}
	resp, err := client.Do(r)

	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, err
	}

	if resp.StatusCode != 200 {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, fmt.Errorf(resp.Status)
	}

	return goquery.NewDocumentFromResponse(resp)
}

func cleanInt(str string) (int64, error) {
	// sometimes it starts with ~
	if strings.HasPrefix(str, "~") {
		str = str[1:]
	}
	// , for thousands etc.
	str = strings.Replace(str, ",", "", -1)
	// finally trim
	str = strings.Trim(str, " ")

	return strconv.ParseInt(str, 10, 64)
}
