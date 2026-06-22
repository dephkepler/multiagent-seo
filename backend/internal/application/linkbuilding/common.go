package linkbuilding

import "errors"

var ErrNoSheet = errors.New("sheet name is required")

type LLMDefaults struct {
	Provider string
	Model    string
}

func pickStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
