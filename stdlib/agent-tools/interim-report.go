package agent_tools

type InterimReport struct {
}

func (i *InterimReport) Name() string {
	return "interim-report"
}

func (i *InterimReport) ContextDescription() string {
	return `use it to report your preliminary results, name: "interim-report", args: "text": "provide all information available on your findings"`
}

func (i *InterimReport) Run(args []string) error {
	return nil
}
