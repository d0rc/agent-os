package cmds

import (
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/batcher"
	"github.com/d0rc/agent-os/server"
	g "github.com/serpapi/google-search-results-golang"
	"sync"
	"time"
)

func ProcessGoogleSearches(request []GoogleSearchRequest, ctx *server.Context) (*ServerResponse, error) {
	results := make([]chan *GoogleSearchResponse, len(request))
	for idx, gsr := range request {
		results[idx] = make(chan *GoogleSearchResponse)
		go func(gsr GoogleSearchRequest, ch chan *GoogleSearchResponse) {
			searchResponse, err := processGoogleSearch(&gsr, ctx)
			if err != nil {
				ctx.Log.Error().Err(err).
					Msgf("Error executing google search request: %v", gsr)
			}

			ch <- searchResponse
		}(gsr, results[idx])
	}

	finalResults := make([]*GoogleSearchResponse, len(request))
	for idx, ch := range results {
		finalResults[idx] = <-ch
	}

	return &ServerResponse{
		GoogleSearchResponse: finalResults,
	}, nil
}

type GoogleSearchCacheRecord struct {
	Id         int64     `db:"id"`
	Keywords   string    `db:"keywords"`
	Lang       string    `db:"lang"`
	Country    string    `db:"country"`
	Location   string    `db:"location"`
	RawContent []byte    `db:"raw_content"`
	CreatedAt  time.Time `db:"created_at"`
	CacheHits  int64     `db:"cache_hits"`
}

var currentSearches = make(map[string][]chan *GoogleSearchResponse)
var currentSearchesLock = sync.RWMutex{}

func processGoogleSearch(gsr *GoogleSearchRequest, ctx *server.Context) (*GoogleSearchResponse, error) {
	cachedSearches := make([]GoogleSearchCacheRecord, 0, 1)
	err := ctx.Storage.Db.GetStructsSlice("query-search-by-keywords", &cachedSearches,
		gsr.Keywords, gsr.Lang, gsr.Country, gsr.Location)

	if len(cachedSearches) > 0 {
		selectedCacheResult := cachedSearches[0]
		result, err := parseRawSearchContentJson(selectedCacheResult.RawContent)
		if err != nil {
			ctx.Log.Error().Err(err).
				Msgf("falling back to new search - error parsing cache data for keywords: %s", gsr.Keywords)
		} else {
			// mark cache hit...!
			searchesBatcher := batcher.NewBatcher("search-cache-hits-batcher", func(ids []int64) error {
				_, err := ctx.Storage.Db.Exec("make-search-cache-hits", ids)
				return err
			}, 256, 500*time.Millisecond)

			ctx.Log.Trace().Msgf("google-search-hit [%d](fg:cyan,mod:bold): [%s](fg:green,mod:bold)", selectedCacheResult.Id, gsr.Keywords)
			searchesBatcher.RunTask(selectedCacheResult.Id)

			// fill the result here....!
			return &GoogleSearchResponse{
				URLSearchInfos: result.OrganicUrs,
				AnswerBox:      result.AnswerBox,
				DownloadedAt:   selectedCacheResult.CreatedAt.Second(),
				SearchAge:      int(time.Since(selectedCacheResult.CreatedAt).Seconds()),
			}, nil
		}
	}

	mapResultsChannel := make(chan *GoogleSearchResponse)
	currentSearchesLock.Lock()
	if _, exists := currentSearches[gsr.Keywords]; exists {
		currentSearches[gsr.Keywords] = append(currentSearches[gsr.Keywords], mapResultsChannel)
		currentSearchesLock.Unlock()

		someResult := <-mapResultsChannel
		if someResult.URLSearchInfos == nil {
			return nil, fmt.Errorf("error running Google search for keywords: %s", gsr.Keywords)
		}
		return someResult, nil
	}
	currentSearches[gsr.Keywords] = make([]chan *GoogleSearchResponse, 0)
	currentSearchesLock.Unlock()

	if gsr.MaxRetries == 0 {
		gsr.MaxRetries = 10
	}

	result := &GoogleSearchCacheRecord{}

	for retryCounter := 0; retryCounter < gsr.MaxRetries; retryCounter++ {
		result, err = executeSearch(gsr, ctx)
		if err == nil {
			break
		}

		time.Sleep(time.Duration(1000) * time.Millisecond)
	}

	if result == nil {
		ctx.Log.Error().Err(err).
			Msgf("[MAX-ATTEMPT-REACHED] error running google search for keywords: %s", gsr.Keywords)
		// also send error to all channels
		currentSearchesLock.Lock()
		for _, ch := range currentSearches[gsr.Keywords] {
			ch <- &GoogleSearchResponse{
				URLSearchInfos: nil,
				AnswerBox:      "",
				DownloadedAt:   -1,
				SearchAge:      0,
			}
		}
		// now delete all these searches
		delete(currentSearches, gsr.Keywords)
		currentSearchesLock.Unlock()
		return nil, fmt.Errorf("error running Google search for keywords: %s", gsr.Keywords)
	}

	// now save results to cache and return
	_, err = ctx.Storage.Db.Exec("save-search-cache-record",
		result.Keywords,
		result.Lang,
		result.Country,
		result.Location,
		result.RawContent,
		time.Now(),
		0)
	if err != nil {
		ctx.Log.Error().Err(err).
			Msgf("error saving cache search record for keywords: %s", gsr.Keywords)
	}

	searchData, err := parseRawSearchContentJson(result.RawContent)
	if err != nil {
		ctx.Log.Error().Err(err).
			Msgf("almost fatal error - failed to parse most recent search result for keywords: %s", gsr.Keywords)
	}

	// send to all channels...!
	finalResponse := &GoogleSearchResponse{
		URLSearchInfos: searchData.OrganicUrs,
		AnswerBox:      searchData.AnswerBox,
		DownloadedAt:   result.CreatedAt.Second(),
		SearchAge:      int(time.Since(result.CreatedAt).Seconds()),
	}
	currentSearchesLock.Lock()
	for _, ch := range currentSearches[gsr.Keywords] {
		ch <- finalResponse
	}
	delete(currentSearches, gsr.Keywords)
	currentSearchesLock.Unlock()

	return finalResponse, nil
}

func executeSearch(gsr *GoogleSearchRequest, ctx *server.Context) (result *GoogleSearchCacheRecord, err error) {
	parameter := map[string]string{
		"q":             gsr.Keywords,
		"location":      gsr.Location,
		"hl":            gsr.Lang,
		"gl":            gsr.Country,
		"google_domain": "google.com",
		"start":         "0",
		"num":           "100",
		"api_key":       ctx.Config.Tools.SerpApi.Token,
	}

	organicUrls := make([]*URLSearchInfo, 0)
	answerBoxText := ""

	search := g.NewGoogleSearch(parameter, ctx.Config.Tools.SerpApi.Token)
	searchResults, err := search.GetJSON()

	if searchResults["organic_results"] != nil {
		organicResults := searchResults["organic_results"].([]interface{})

		for _, organicResultsUrl := range organicResults {
			if organicResultsUrl.(map[string]interface{})["link"] == nil ||
				organicResultsUrl.(map[string]interface{})["title"] == nil ||
				organicResultsUrl.(map[string]interface{})["snippet"] == nil {

				continue
			}

			organicUrls = append(organicUrls, &URLSearchInfo{
				URL:     organicResultsUrl.(map[string]interface{})["link"].(string),
				Title:   organicResultsUrl.(map[string]interface{})["title"].(string),
				Snippet: organicResultsUrl.(map[string]interface{})["snippet"].(string),
			})
		}
	}

	if searchResults["answer_box"] != nil {
		answerBody := searchResults["answer_box"].(map[string]interface{})["answerBody"]

		if answerBody != nil {
			answerBoxText = answerBody.(string)
		}
	}

	rawContent, err := generateRawSearchContentJson(organicUrls, answerBoxText)
	if err != nil {
		return nil, err
	}

	cmdSearchResults := &GoogleSearchCacheRecord{
		Keywords:   gsr.Keywords,
		Lang:       gsr.Lang,
		Country:    gsr.Country,
		Location:   gsr.Location,
		RawContent: rawContent,
		CreatedAt:  time.Now(),
	}

	return cmdSearchResults, nil
}

type searchResultsJson struct {
	OrganicUrs []*URLSearchInfo
	AnswerBox  string
}

func generateRawSearchContentJson(organicUrls []*URLSearchInfo, answerBoxText string) ([]byte, error) {
	searchResults := searchResultsJson{
		OrganicUrs: organicUrls,
		AnswerBox:  answerBoxText,
	}

	return json.Marshal(searchResults)
}

func parseRawSearchContentJson(rawContent []byte) (*searchResultsJson, error) {
	var searchResults searchResultsJson

	err := json.Unmarshal(rawContent, &searchResults)
	if err != nil {
		return nil, err
	}

	return &searchResults, nil
}
