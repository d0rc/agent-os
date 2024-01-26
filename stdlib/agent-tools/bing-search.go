package agent_tools

type BingSearch struct {
}

func (b *BingSearch) Name() string {
	return "bing-search"
}

func (b *BingSearch) ContextDescription() string {
	return `use it to search Bing, name: "bing-search", args: "keywords": "search keywords or question"`
}

func (b *BingSearch) Run(args []string) error {
	return nil
}
