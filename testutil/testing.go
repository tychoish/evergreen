package testutil

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// run the integration tests
var runAllTests = flag.Bool("evergreen.all", false, "Run integration tests")

// HandleTestingErr catches errors that we do not want to treat
// as relevant a goconvey statement. HandleTestingErr is used
// to terminate unit tests that fail for reasons that are orthogonal to
// the test (filesystem errors, database errors, etc).
func HandleTestingErr(err error, t *testing.T, format string, a ...interface{}) {
	if err != nil {
		_, file, line, ok := runtime.Caller(1)
		if ok {
			t.Fatalf("%v:%v: %q: %v", file, line, fmt.Sprintf(format, a), err)
		} else {
			t.Fatalf("%q: %v", fmt.Sprintf(format, a), err)
		}
	}
}

// GetDirectoryOfFile returns the path to of the file that calling
// this function. Use this to ensure that references to testdata and
// other file system locations in tests are not dependent on the working
// directory of the "go test" invocation.
func GetDirectoryOfFile() string {
	_, file, _, _ := runtime.Caller(1)

	return filepath.Dir(file)
}

// SkipTestUnlessAll skipps the current test
func SkipTestUnlessAll(t *testing.T, testName string) {
	if !(*runAllTests) || !strings.Contains(os.Getenv("TEST_ARGS"), "evergreen.all") {
		t.Skip(fmt.Sprintf("skipping %v because 'evergreen.all' is not specified...",
			testName))
	}
}
