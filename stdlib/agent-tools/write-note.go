package agent_tools

type WriteNote struct {
}

func (w *WriteNote) Name() string {
	return "write-note"
}

func (w *WriteNote) ContextDescription() string {
	return `use it to take a note, name: "write-note", args: "section": "section name", "text": "text"`
}

func (w *WriteNote) Run(args []string) error {
	return nil
}
