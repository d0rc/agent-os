package agent_tools

type ReadNote struct {
}

func (r *ReadNote) Name() string {
	return "read-note"
}

func (r *ReadNote) ContextDescription() string {
	return `use it to read a note, name: "read-note", args: "section": "section name"`
}

func (r *ReadNote) Run(args []string) error {
	return nil
}
