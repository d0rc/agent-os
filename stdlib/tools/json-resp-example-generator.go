package tools

import (
	"fmt"
	"strings"
)

func RenderJsonString(structure []MapKV, buffer *strings.Builder, depth int) string {
	if depth == 0 {
		buffer.WriteString("{\n")
		RenderJsonString(structure, buffer, depth+1)
		buffer.WriteString("}\n")
	} else {
		for _, kv := range structure {
			if kv.Value != nil {
				buffer.WriteString(fmt.Sprintf("%s\"%s\": \"%s\",\n", strings.Repeat("\t", depth), kv.Key, kv.Value))
			} else {
				buffer.WriteString(fmt.Sprintf("%s\"%s\": {\n", strings.Repeat("\t", depth), kv.Key))
				RenderJsonString(kv.InnerMap, buffer, depth+1)
				buffer.WriteString(fmt.Sprintf("%s},\n", strings.Repeat("\t", depth)))
			}
		}
	}

	return buffer.String()
}
