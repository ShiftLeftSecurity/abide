package abide

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// Assertable represents an object that can be asserted.
type Assertable interface {
	String() string
}

// Assert asserts the value of an object with implements Assertable.
func Assert(t *testing.T, id string, a Assertable) {
	data := a.String()
	createOrUpdateSnapshot(t, id, data, SnapshotGeneric)
}

// AssertReader asserts the value of an io.Reader.
func AssertReader(t *testing.T, id string, r io.Reader) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	createOrUpdateSnapshot(t, id, string(data), SnapshotGeneric)
}

func createOrUpdateSnapshot(t *testing.T, id, data string, format SnapshotType) {
	var err error
	snapshot := getSnapshot(snapshotID(id))

	if snapshot == nil {
		if !args.shouldUpdate {
			t.Error(newSnapshotMessage(id, data))
			return
		}

		fmt.Printf("Creating snapshot `%s`\n", id)
		snapshot, err = createSnapshot(snapshotID(id), data)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.evaluated = true
		return
	}

	snapshot.evaluated = true
	var diff string
	switch format {
	case SnapshotHTTPRespJSON:
		diff = compareResultsHTTPRequestJSON(t, snapshot.value, strings.TrimSpace(data))
	default:
		diff = compareResults(t, id, snapshot.value, strings.TrimSpace(data))
	}

	if diff != "" {
		if snapshot != nil && args.shouldUpdate {
			fmt.Printf("Updating snapshot `%s`\n", id)
			_, err = updateSnapshot(snapshotID(id), data)
			if err != nil {
				t.Fatal(err)
			}
			return
		}

		t.Error(didNotMatchMessage(id, diff))
		return
	}
}

func compareResults(t *testing.T, id, existing, new string) string {
	c, err := getConfig()
	if err != nil {
		t.Fatal(err)
	}

	if c != nil && c.UnifiedDiff {
		edits := myers.ComputeEdits(span.URIFromPath(id), existing, new)
		diff := gotextdiff.ToUnified("a.txt", "b.txt", existing, edits)
		return fmt.Sprint(diff)
	}

	dmp := diffmatchpatch.New()
	dmp.PatchMargin = 20
	allDiffs := dmp.DiffMain(existing, new, false)
	var nonEqualDiffs []diffmatchpatch.Diff
	for _, diff := range allDiffs {
		if diff.Type != diffmatchpatch.DiffEqual {
			nonEqualDiffs = append(nonEqualDiffs, diff)
		}
	}

	if len(nonEqualDiffs) == 0 {
		return ""
	}

	return dmp.DiffPrettyText(allDiffs)
}

func didNotMatchMessage(id, diff string) string {
	msg := "\n\n## Existing snapshot does not match results...\n"
	msg += "## \"" + id + "\"\n\n"
	msg += diff
	msg += "\n\n"
	msg += "If this change was intentional, run tests again, $ go test -v -- -u\n"
	return msg
}

func newSnapshotMessage(id, body string) string {
	msg := "\n\n## New snapshot found...\n"
	msg += "## \"" + id + "\"\n\n"
	msg += body
	msg += "\n\n"
	msg += "To save, run tests again, $ go test -v -- -u\n"
	return msg
}
