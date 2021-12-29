package abide

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
	"testing"

	"github.com/nsf/jsondiff"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// AssertHTTPResponse asserts the value of an http.Response.
func AssertHTTPResponse(t *testing.T, id string, w *http.Response) {
	body, err := httputil.DumpResponse(w, true)
	if err != nil {
		t.Fatal(err)
	}

	assertHTTP(t, id, body, contentTypeIsJSON(w.Header.Get("Content-Type")))
}

// AssertHTTPRequestOut asserts the value of an http.Request.
// Intended for use when testing outgoing client requests
// See https://golang.org/pkg/net/http/httputil/#DumpRequestOut for more
func AssertHTTPRequestOut(t *testing.T, id string, r *http.Request) {
	body, err := httputil.DumpRequestOut(r, true)
	if err != nil {
		t.Fatal(err)
	}

	assertHTTP(t, id, body, contentTypeIsJSON(r.Header.Get("Content-Type")))
}

type httpRequest struct {
	header []string
	body   string
}

// headerDump returns the request header in form of a string to be compared.
func (h *httpRequest) headerDump() string {
	return strings.Join(h.header, "\n")
}

// byteBody is a convenience method to cast body to a useful type.
func (h *httpRequest) byteBody() []byte {
	return []byte(h.body)
}

// dump puts the request in form of a string to be written in the snapshot
func (h *httpRequest) dump() string {
	return fmt.Sprintf("%s\n\n%s", h.headerDump(), h.body)
}

func (h *httpRequest) jsonBodyCleanup(c *config) error {
	jsonStr := h.body
	if strings.TrimSpace(jsonStr) == "" {
		return nil
	}

	var jsonIface map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &jsonIface)
	if err != nil {
		return err
	}

	// Clean/update json based on config.
	if c != nil {
		for k, v := range c.Defaults {
			jsonIface = updateKeyValuesInMap(k, v, jsonIface)
		}
	}

	out, err := json.MarshalIndent(jsonIface, "", "  ")
	if err != nil {
		return err
	}

	h.body = string(out)
	return nil
}

func (h *httpRequest) configCleanup(c *config) {
	if c == nil {
		return
	}
	// empty line identifies the end of the HTTP header
	for i, line := range h.header {
		headerItem := strings.Split(line, ":")
		key := strings.TrimSpace(headerItem[0])
		if def, ok := c.Defaults[key]; ok {
			h.header[i] = fmt.Sprintf("%s: %s", headerItem[0], def)
		}
	}
}

func plainToInternalRequest(requestDump []byte) *httpRequest {
	data := string(requestDump)
	lines := strings.Split(strings.TrimSpace(data), "\n")
	h := httpRequest{}
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			break
		}
		h.header = append(h.header, lines[i])
	}
	headerEnd := len(h.header) // this is the place where the empty line resides
	h.body = strings.TrimSpace(strings.Join(lines[headerEnd:], "\n"))
	return &h
}

// AssertHTTPRequest asserts the value of an http.Request.
// Intended for use when testing incoming client requests
// See https://golang.org/pkg/net/http/httputil/#DumpRequest for more
func AssertHTTPRequest(t *testing.T, id string, r *http.Request) {
	body, err := httputil.DumpRequest(r, true)
	if err != nil {
		t.Fatal(err)
	}

	assertHTTP(t, id, body, contentTypeIsJSON(r.Header.Get("Content-Type")))
}

// assertHTTP processes the body, this handling happens twice because there is more refactor to do but
// it is better to have it in place for the future.
func assertHTTP(t *testing.T, id string, body []byte, isJSON bool) {
	c, err := getConfig()
	if err != nil {
		t.Fatal(err)
	}

	h := plainToInternalRequest(body)
	h.configCleanup(c)

	snapshotType := SnapshotGeneric

	// If the response body is JSON, indent.
	if isJSON {
		if err := h.jsonBodyCleanup(c); err != nil {
			t.Fatal(err)
		}
		snapshotType = SnapshotHTTPRespJSON
	}

	createOrUpdateSnapshot(t, id, h.dump(), snapshotType)
}

func contentTypeIsJSON(contentType string) bool {
	contentTypeParts := strings.Split(contentType, ";")
	firstPart := contentTypeParts[0]

	isPlainJSON := firstPart == "application/json"
	if isPlainJSON {
		return isPlainJSON
	}

	isVendor := strings.HasPrefix(firstPart, "application/vnd.")

	isJSON := strings.HasSuffix(firstPart, "+json")

	return isVendor && isJSON
}

func compareResultsHTTPRequestJSON(t *testing.T, existing, new string) string {
	existingR := plainToInternalRequest([]byte(existing))
	newR := plainToInternalRequest([]byte(new))
	c, err := getConfig()
	if err != nil {
		t.Fatal(err)
	}
	existingR.configCleanup(c)
	newR.configCleanup(c)
	existingR.jsonBodyCleanup(c)
	newR.jsonBodyCleanup(c)
	// let us compare the headers in the old school ways
	dmp := diffmatchpatch.New()
	dmp.PatchMargin = 20
	allDiffs := dmp.DiffMain(existingR.headerDump(), newR.headerDump(), false)
	var nonEqualDiffs []diffmatchpatch.Diff
	for _, diff := range allDiffs {
		if diff.Type != diffmatchpatch.DiffEqual {
			nonEqualDiffs = append(nonEqualDiffs, diff)
		}
	}

	var diffSoFar string
	if len(nonEqualDiffs) != 0 {
		diffSoFar = dmp.DiffPrettyText(allDiffs)
	}

	opts := jsondiff.DefaultConsoleOptions()

	jsonDifference, explanation := jsondiff.Compare(existingR.byteBody(), newR.byteBody(), &opts)
	if jsonDifference == jsondiff.FullMatch {
		return diffSoFar
	}
	diffSoFar += "\n"
	switch jsonDifference {
	case jsondiff.SupersetMatch, jsondiff.NoMatch:
		diffSoFar += explanation
	case jsondiff.FirstArgIsInvalidJson:
		diffSoFar += "ERROR: Existing body is not valid JSON"
	case jsondiff.SecondArgIsInvalidJson:
		diffSoFar += "ERROR: New body is not valid JSON"
	case jsondiff.BothArgsAreInvalidJson:
		if len(existingR.body) == len(newR.body) && len(newR.body) == 0 {
			// empty
			return diffSoFar
		}
		diffSoFar += "ERROR: Neither Existing nor New bodies are valid JSON\n"
		diffSoFar += existingR.dump()
		diffSoFar += "\n"
		diffSoFar += newR.dump()

	}
	return diffSoFar
}
