package cmds

import (
	"fmt"
	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"github.com/d0rc/agent-os/server"
	"github.com/logrusorgru/aurora"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var maxThreads = make(chan struct{}, 32)

func ProcessPageRequests(request []GetPageRequest, ctx *server.Context) (*ServerResponse, error) {
	// there's no way to speed up page downloads except to run these requests in parallel
	results := make([]chan *GetPageResponse, len(request))
	for idx, pr := range request {
		results[idx] = make(chan *GetPageResponse, 1)
		go func(pr GetPageRequest, ch chan *GetPageResponse) {
			maxThreads <- struct{}{}
			defer func() {
				<-maxThreads
			}()

			pageResponse, err := processPageRequest(pr, ctx)
			if err != nil {
				// since, we've got here - it's a fatal error for this request
				// need to return error to the one asking...
				ctx.Log.Error().Err(err).
					Msgf("Error processing page request: %s", pr.Url)
			}
			ch <- pageResponse
		}(pr, results[idx])
	}

	finalResults := make([]*GetPageResponse, len(request))
	// now, reading the response
	for idx, ch := range results {
		finalResults[idx] = <-ch
	}

	return &ServerResponse{
		GetPageResponse: finalResults,
	}, nil
}

type PageCacheRecord struct {
	Id         int64     `db:"id"`
	Url        string    `db:"url"`
	RawContent []byte    `db:"raw_content"`
	CreatedAt  time.Time `db:"created_at"`
	CacheHits  uint      `db:"cache_hits"`
	StatusCode uint      `db:"status_code"`
}

func processPageRequest(pr GetPageRequest, ctx *server.Context) (*GetPageResponse, error) {
	// let's read all the pages we've seen from this url and pick latest
	cachedPage := make([]PageCacheRecord, 0, 1)
	err := ctx.Storage.Db.GetStructsSlice("query-page-cache", &cachedPage, pr.Url)
	if err != nil {
		return nil, fmt.Errorf("error running query-page-cache: %v", err)
	}

	if len(cachedPage) > 0 {
		// we've got the page, let's see if it's not too old
		if pr.MaxAge == 0 {
			pr.MaxAge = int((365 * 24 * time.Hour).Seconds())
		}

		if time.Since(cachedPage[0].CreatedAt).Seconds() < float64(pr.MaxAge) {
			// it's a cache hit, let's mark it and exit
			_, err = ctx.Storage.Db.Exec("make-page-cache-hit", cachedPage[0].Id)
			if err != nil {
				ctx.Log.Error().Err(err).Msgf("error marking page cache hit: %v", cachedPage[0].Id)
			}

			pageCacheRecord := translateCacheRecordToClientResponse(&cachedPage[0], ctx)
			pageCacheRecord.Question = pr.Question
			pageCacheRecord.Url = pr.Url
			return pageCacheRecord, nil
		}
	}

	// if we've got here, page in cache either not exists
	// or too old, so let's fetch a new one
	if pr.MaxRetries == 0 {
		pr.MaxRetries = 10
	}

	var pageCacheRecord *PageCacheRecord
	for retryCounter := 0; retryCounter < pr.MaxRetries; retryCounter++ {
		pageCacheRecord, err = downloadPage(pr, ctx)
		if err != nil {
			ctx.Log.Warn().Err(err).
				Int("retry-counter", retryCounter).
				Msgf("error loading page from url: %s", pr.Url)
		}

		if pageCacheRecord != nil {
			break
		}

		time.Sleep(1 * time.Second)
	}

	if pageCacheRecord == nil {
		ctx.Log.Error().Err(err).
			Msgf("[MAX-ATTEMPT-REACHED] error loading page from url: %s", pr.Url)
		return nil, fmt.Errorf("error loading page from url: %s", pr.Url)
	}

	// saving cache record to database
	res, err := ctx.Storage.Db.Exec("save-page-cache-record",
		pageCacheRecord.Url,
		pageCacheRecord.RawContent,
		pageCacheRecord.CreatedAt,
		pageCacheRecord.CacheHits,
		pageCacheRecord.StatusCode)
	if err != nil {
		ctx.Log.Error().Err(err).
			Msgf("error saving page cache record to database: %s", pr.Url)
	} else {
		pageCacheRecord.Id, err = res.LastInsertId()
	}

	return translateCacheRecordToClientResponse(pageCacheRecord, ctx), nil
}

func translateCacheRecordToClientResponse(cachedPage *PageCacheRecord, ctx *server.Context) *GetPageResponse {
	mdBody, err := renderMarkdown(string(cachedPage.RawContent))
	if err != nil {
		ctx.Log.Error().Err(err).Msgf("error rendering markdown for page cache id: %v",
			cachedPage.Id)
	}

	pageResponse := &GetPageResponse{
		StatusCode:   cachedPage.StatusCode,
		Markdown:     mdBody,
		RawData:      string(cachedPage.RawContent),
		DownloadedAt: cachedPage.CreatedAt.Second(),
		PageAge:      int(time.Since(cachedPage.CreatedAt).Seconds()),
	}
	return pageResponse
}

func downloadPage(pr GetPageRequest, ctx *server.Context) (*PageCacheRecord, error) {
	// somewhere here, we need to make sure we're the only
	// process which downloads the URL in the way requested
	// and if there's someone doing the same, just wait for his result
	client := http.Client{Timeout: time.Duration(pr.TimeOut) * time.Second}

	escapedUrl := url.QueryEscape(pr.Url)
	finalUrl := fmt.Sprintf("https://api.crawlbase.com/?token=%s&url=", ctx.Config.Tools.ProxyCrawl.Token) + escapedUrl

	ts := time.Now()
	result, err := client.Get(finalUrl)
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}

	ctx.Log.Info().Msgf("Downloaded %s in %s\n",
		aurora.Cyan(noLongerThen(pr.Url, 45)), aurora.BrightCyan(time.Since(ts)))
	return &PageCacheRecord{
		Id:         0,
		Url:        pr.Url,
		RawContent: body,
		CreatedAt:  time.Now(),
		CacheHits:  0,
		StatusCode: uint(result.StatusCode),
	}, nil
}

func noLongerThen(u string, i int) string {
	if len(u) > i {
		return u[:i] + "..."
	}

	return u
}

func ignoreDataUrls(content string, selec *goquery.Selection, opt *md.Options) *string {
	if strings.HasPrefix(content, "data:") {
		emptyString := ""
		return &emptyString
	}

	return nil
}

func renderMarkdown(rawData string) (string, error) {
	convertor := md.NewConverter("", true, nil)
	convertor.AddRules(md.Rule{
		Filter:      []string{"img"},
		Replacement: ignoreDataUrls,
	})

	return convertor.ConvertString(rawData)
}
