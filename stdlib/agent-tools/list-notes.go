package agent_tools

type ListNotes struct {
}

func (l *ListNotes) Name() string {
	return "list-notes"
}

func (l *ListNotes) ContextDescription() string {
	return `use it get names of last 50 notes you've made`
}

func (l *ListNotes) Run(args []string) error {
	return nil
}
