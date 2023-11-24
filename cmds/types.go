package cmds

import borrow_engine "github.com/d0rc/agent-os/borrow-engine"

type GetPageRequest struct {
	Url           string `json:"url"`
	TimeOut       int    `json:"time-out"`
	MaxRetries    int    `json:"max-retries"`
	MaxAge        int    `json:"max-age"`
	Question      string `json:"question"`
	ReturnSummary bool   `json:"return-summary"`
}

type GetPageResponse struct {
	StatusCode   uint   `json:"status-code"`
	Markdown     string `json:"markdown"`
	RawData      string `json:"raw-data"`
	DownloadedAt int    `json:"downloaded-at"`
	PageAge      int    `json:"page-age"`
	Question     string `json:"question"`
	Url          string `json:"url"`
}

type GoogleSearchRequest struct {
	Keywords   string `json:"keywords"`
	Lang       string `json:"lang"`
	Country    string `json:"country"`
	Location   string `json:"location"`
	MaxAge     int    `json:"max-age"`
	MaxRetries int    `json:"max-retries"`
}

type URLSearchInfo struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

type GoogleSearchResponse struct {
	AnswerBox      string           `json:"answer-box"`
	URLSearchInfos []*URLSearchInfo `json:"url-search-infos"`
	DownloadedAt   int              `json:"downloaded-at"`
	SearchAge      int              `json:"search-age"`
}

type GetCompletionRequest struct {
	Model       string   `json:"model-mask"` // * - any model
	RawPrompt   string   `json:"raw-prompt"` //
	Temperature float32  `json:"temperature"`
	StopTokens  []string `json:"stop-tokens"`
	MinResults  int      `json:"min-results"`
	MaxResults  int      `json:"max-results"` // default = 100
	BestOf      int      `json:"best-of"`
}

type GetEmbeddingsRequest struct {
	Model           string `json:"model-mask"` // * - any model
	RawPrompt       string `json:"raw-prompt"` //
	MetaNamespace   string `json:"meta-namespace"`
	MetaNamespaceId int64  `json:"meta-namespace-id"`
}

type GetEmbeddingsResponse struct {
	Embeddings []float64 `json:"embeddings"`
	TextHash   string    `json:"text-hash"`
	Model      string    `json:"model"`
	Text       string    `json:"text"`
}

type GetCompletionResponse struct {
	Choices []string `json:"choices"`
}

type GetCacheRecord struct {
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
}

type GetCacheRecordResponse struct {
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
	Value     []byte `json:"value"`
}

type SetCacheRecord struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Value     []byte `json:"value"`
}

type SetCacheRecordResponse struct {
	Done bool `json:"done"`
}

type ClientRequest struct {
	Tags                  []string                  `json:"tags"`
	ProcessName           string                    `json:"process-name"`
	Priority              borrow_engine.JobPriority `json:"priority"`
	GetPageRequests       []GetPageRequest          `json:"get-page-request"`
	GoogleSearchRequests  []GoogleSearchRequest     `json:"google-search-request"`
	GetCompletionRequests []GetCompletionRequest    `json:"get-completion-requests"`
	GetEmbeddingsRequests []GetEmbeddingsRequest    `json:"get-embeddings-requests"`
	CorrelationId         string                    `json:"correlation-id"`
	SpecialCaseResponse   string                    `json:"special-case-response"`
	GetCacheRecords       []GetCacheRecord          `json:"get-cache-records"`
	SetCacheRecords       []SetCacheRecord          `json:"set-cache-records"`
}

type ServerResponse struct {
	GoogleSearchResponse  []*GoogleSearchResponse   `json:"google-search-response"`
	GetPageResponse       []*GetPageResponse        `json:"get-page-response"`
	GetCompletionResponse []*GetCompletionResponse  `json:"get-completion-response"`
	GetEmbeddingsResponse []*GetEmbeddingsResponse  `json:"get-embeddings-response"`
	GetCacheRecords       []*GetCacheRecordResponse `json:"get-cache-records"`
	SetCacheRecords       []*SetCacheRecordResponse `json:"set-cache-records"`
	CorrelationId         string                    `json:"correlation-id"`
	SpecialCaseResponse   string                    `json:"special-case-response"`
}
