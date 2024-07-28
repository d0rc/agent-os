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
		for idx, kv := range structure {
			if kv.Value != nil {
				buffer.WriteString(fmt.Sprintf("%s\"%s\": \"%s\"", strings.Repeat("\t", depth), kv.Key, kv.Value))
				if idx == len(structure)-1 {
					buffer.WriteString("\n")
				} else {
					buffer.WriteString(",\n")
				}
			} else {
				buffer.WriteString(fmt.Sprintf("\n%s\"%s\": {\n", strings.Repeat("\t", depth), kv.Key))
				RenderJsonString(kv.InnerMap, buffer, depth+1)

				if len(structure)-1 == idx {
					buffer.WriteString(fmt.Sprintf("\n%s}\n", strings.Repeat("\t", depth)))
				} else {
					buffer.WriteString(fmt.Sprintf("\n%s},\n", strings.Repeat("\t", depth)))
				}
			}
		}

	}

	return buffer.String()
}
