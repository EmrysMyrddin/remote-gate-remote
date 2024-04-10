package views

import "github.com/a-h/templ"

func mergeAttributes(attrs []templ.Attributes) templ.Attributes {
	if attrs == nil {
		return nil
	}
	if len(attrs) == 1 {
		return attrs[0]
	}
	merged := templ.Attributes{}
	for _, attr := range attrs {
		for k, v := range attr {
			merged[k] = v
		}
	}
	return merged
}
