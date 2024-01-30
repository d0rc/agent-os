package agent_tools

import (
	"strings"
)

/*

   Available tools:

   - browse-site - use it to browses specific URL, name: "browse-site", args: "url": "url", "question": "question to look answer for";

   - bing-search - use it to search Bing, name: "bing-search", args: "keywords": "search keywords or question";

   - read-note - use it to read a note, name: "read-note", args: "section": "section name";

   - write-note - use it to take a note, name: "write-note", args: "section": "section name", "text": "text";

   - list-notes - use it get names of last 50 notes you've made;

   - interim-report - use it to report your preliminary results, name: "interim-report", args: "text": "provide all information available on your findings";

   - final-report - use it to deliver the solution, name: "final-report", args: "text": "solution with all the useful details, including URLs, names, section titles, etc.".


*/

type AgentTool interface {
	Name() string
	Run(args []string) error
	ContextDescription() string
}

var allTools = []AgentTool{
	&BrowseSite{},
	&BingSearch{},
	&ReadNote{},
	&WriteNote{},
	&ListNotes{},
	&InterimReport{},
	&FinalReport{},
}

func GetToolsSelection(exclude []string) []AgentTool {
	resultingTools := make([]AgentTool, 0)

	for _, tool := range allTools {
		if !contains(exclude, tool.Name()) {
			resultingTools = append(resultingTools, tool)
		}
	}

	return resultingTools
}

func GetContextDescription(tools []AgentTool) string {
	result := strings.Builder{}
	result.WriteString("Available tools:\n")
	for idx, tool := range tools {
		result.WriteString(tool.Name())
		result.WriteString(" - ")
		result.WriteString(tool.ContextDescription())
		if idx < len(tools)-1 {
			result.WriteString(";\n")
		} else {
			result.WriteString(".\n")
		}
	}
	result.WriteString("\n")

	return result.String()
}

func contains(exclude []string, name string) bool {
	contained := false
	for _, item := range exclude {
		if item == name {
			contained = true
			break
		}
	}

	return contained
}
