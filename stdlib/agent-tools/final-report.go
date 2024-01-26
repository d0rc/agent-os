package agent_tools

type FinalReport struct {
}

func (f *FinalReport) Name() string {
	return "final-report"
}

func (f *FinalReport) ContextDescription() string {
	return `use it to deliver the solution, name: "final-report", args: "text": "solution with all the useful details, including URLs, names, section titles, etc.".`
}

func (f *FinalReport) Run(args []string) error {
	return nil
}
