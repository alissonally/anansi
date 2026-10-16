package sqllex

import (
	"strings"
	"testing"
)

func bigScript() string {
	var b strings.Builder
	for b.Len() < 4<<20 {
		b.WriteString("INSERT INTO t VALUES ('ação; teste', 12, NULL);\n")
	}
	return b.String()
}

func BenchmarkTokenize4MB(b *testing.B) {
	s := bigScript()
	for i := 0; i < b.N; i++ {
		Tokenize(s)
	}
}

func BenchmarkSplit4MB(b *testing.B) {
	s := bigScript()
	for i := 0; i < b.N; i++ {
		Split(s)
	}
}
