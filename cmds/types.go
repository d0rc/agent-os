package cmds

type GetPageRequest struct {
	Url        string `json:"url"`
	TimeOut    int    `json:"time-out"`
	MaxRetries int    `json:"max-retries"`
	MaxAge     int    `json:"max-age"`
}

type GetPageResponse struct {
	StatusCode   uint   `json:"status-code"`
	Markdown     string `json:"markdown"`
	RawData      string `json:"raw-data"`
	DownloadedAt int    `json:"downloaded-at"`
	PageAge      int    `json:"page-age"`
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

type GetCompletionResponse struct {
	Choices []string `json:"choices"`
}

type ClientRequest struct {
	Tags                  []string               `json:"tags"`
	GetPageRequests       []GetPageRequest       `json:"get-page-request"`
	GoogleSearchRequests  []GoogleSearchRequest  `json:"google-search-request"`
	GetCompletionRequests []GetCompletionRequest `json:"get-completion-requests"`
}

type ServerResponse struct {
	GoogleSearchResponse  []*GoogleSearchResponse  `json:"google-search-response"`
	GetPageResponse       []*GetPageResponse       `json:"get-page-response"`
	GetCompletionResponse []*GetCompletionResponse `json:"get-completion-response"`
}
