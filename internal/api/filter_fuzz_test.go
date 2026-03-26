package api

import (
	"strings"
	"testing"
)

func FuzzParseFilter(f *testing.F) {
	tbl := filterTestTable()

	seeds := []string{
		"name='Alice'",
		"status IN ('active','inactive')",
		`name='it\'s'`,
		"name='unterminated",
		"name=$1",
		strings.Repeat("(", maxFilterDepth) + "id=1" + strings.Repeat(")", maxFilterDepth),
		strings.Repeat("(", maxFilterDepth+1) + "id=1" + strings.Repeat(")", maxFilterDepth+1),
		"status IN ('a','b','c'",
		"age>-",
		"'",
		"'abc",
		"id=1 &&",
		"名字='値'",
		"name='line\nbreak'",
		"name='tab\tvalue'",
		"name='\x00'",
		"\x00\x01\x02id=1",
		"((((((((((id=1",
		"status IN (",
		"",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip()
		}

		_, _ = tokenize(input)
		_, _, _ = parseFilter(tbl, input)
	})
}
