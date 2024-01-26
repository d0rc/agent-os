package agent_tools

type BrowseSite struct {
}

func (b *BrowseSite) Name() string {
	return "browse-site"
}

func (b *BrowseSite) ContextDescription() string {
	return `use it to browses specific URL, name: "browse-site", args: "url": "url", "question": "question to look answer for"`
}

func (b *BrowseSite) Run(args []string) error {
	return nil
}
