// Reddit audiences crawler
// Rémy Mathieu © 2016
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/remeh/reddit-audiences/app"

	"github.com/gorilla/mux"
)

type TodayHandler struct {
	App *app.App
}

func (c TodayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	subreddit := vars["subreddit"]
	if len(subreddit) == 0 {
		w.WriteHeader(400)
		return
	}

	data, err := c.getData(subreddit)
	if err != nil {
		log.Println("err:", err.Error())
		w.WriteHeader(500)
		return
	}

	buff, err := json.Marshal(data)
	if err != nil {
		log.Println("err:", err.Error())
		w.WriteHeader(500)
		return
	}
	w.Write(buff)

}

func (c TodayHandler) getData(subreddit string) ([]app.Audience, error) {
	var start, end time.Time

	start = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
	end = time.Now()

	return c.App.DB().FindAudiencesInterval(subreddit, start, end)
}