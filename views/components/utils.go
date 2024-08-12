package components

import "github.com/a-h/templ"

func expect1(attrs []templ.Attributes) templ.Attributes {
	if len(attrs) == 0 {
		return nil
	}
	if len(attrs) == 1 {
		return attrs[0]
	}
	panic("attr called with multiple attributes")
}

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
