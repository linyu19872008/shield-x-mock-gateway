package cli

import (
	"flag"

	"github.com/linyu19872008/shield-x-mock-gateway/internal/conformance"
)

// Conformance runs the template-conformance vectors.
func Conformance(args []string) int {
	fs := flag.NewFlagSet("conformance", flag.ContinueOnError)
	vectors := fs.String("vectors", "./testdata/template-conformance/cases.json", "测试向量文件路径")
	verbose := fs.Bool("v", false, "输出每条用例的说明")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return conformance.Run(*vectors, *verbose)
}
