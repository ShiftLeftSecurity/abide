package abide

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	args         *arguments
	allSnapshots snapshots
	allSnapMutex sync.Mutex
)

var (
	// SnapshotsDir is the directory snapshots will be read to & written from
	// relative directories are resolved to present-working-directory of the executing process
	SnapshotsDir = "__snapshots__"
	// snapshotsLoaded
	snapshotsLoaded = sync.Once{}
)

const (
	// snapshotExt is the file extension to use for a collection of snapshot records
	snapshotExt = ".snapshot"
	// snapshotSeparator deliniates records in the snapshots, not externally settable
	snapshotSeparator = "/* snapshot: "
)

// SnapshotType is the type of snapshot being captured
type SnapshotType string

const (
	// SnapshotGeneric represents a snapshot whose contents we assume have no known format.
	SnapshotGeneric SnapshotType = ""
	// SnapshotHTTPRespJSON represents a snapshot whose contents are an HTTP response with content type JSON.
	SnapshotHTTPRespJSON SnapshotType = "HTTPContentTypeJSON"
)

func init() {
	// Get arguments
	args = getArguments()
}

// Cleanup is an optional method which will execute cleanup operations
// affiliated with abide testing, such as pruning snapshots.
func Cleanup() error {
	for _, s := range allSnapshots {
		if !s.evaluated && args.shouldUpdate && !args.singleRun {
			s.shouldRemove = true
			fmt.Printf("Removing unused snapshot `%s`\n", s.id)
		}
	}

	return allSnapshots.save()
}

// CleanupOrFail is an optional method which will behave like
// Cleanup() if the `-u` flag was given, but which returns an error if
// `-u` was not given and there were things to clean up.
func CleanupOrFail() error {
	if args.singleRun {
		return nil
	}
	if args.shouldUpdate {
		return Cleanup()
	}

	failed := 0
	for _, s := range allSnapshots {
		if !s.evaluated {
			failed++
			fmt.Fprintf(os.Stderr, "Unused snapshot `%s`\n", s.id)
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d unused snapshots", failed)
	}

	return nil
}

// snapshotID represents the unique identifier for a snapshot.
type snapshotID string

// isValid verifies whether the snapshotID is valid. An
// identifier is considered invalid if it is already in use
// or it is malformed.
func (s *snapshotID) isValid() bool {
	return true
}

// snapshot represents the expected value of a test, identified by an id.
type snapshot struct {
	id           snapshotID
	value        string
	path         string
	evaluated    bool
	shouldRemove bool
}

// snapshots represents a map of snapshots by id.
type snapshots map[snapshotID]*snapshot

// save writes all snapshots to their designated files.
func (s snapshots) save() error {
	snapshotsByPath := map[string][]*snapshot{}
	for _, snap := range s {
		_, ok := snapshotsByPath[snap.path]
		if !ok {
			snapshotsByPath[snap.path] = []*snapshot{}
		}
		snapshotsByPath[snap.path] = append(snapshotsByPath[snap.path], snap)
	}

	for path, snaps := range snapshotsByPath {
		if path == "" {
			continue
		}

		snapshotMap := snapshots{}
		for _, snap := range snaps {
			if snap.shouldRemove {
				continue
			}
			snapshotMap[snap.id] = snap
		}
		data, err := encode(snapshotMap)
		if err != nil {
			return err
		}

		err = ioutil.WriteFile(path, data, 0666)
		if err != nil {
			return err
		}
	}

	return nil
}

// decode decides a slice of bytes to retrieve a Snapshots object.
func decode(data []byte) (snapshots, error) {
	snaps := make(snapshots)

	snapshotsStr := strings.Split(string(data), snapshotSeparator)
	for _, s := range snapshotsStr {
		if s == "" {
			continue
		}

		components := strings.SplitAfterN(s, "\n", 2)
		id := snapshotID(strings.TrimSuffix(components[0], " */\n"))
		val := strings.TrimSpace(components[1])
		snaps[id] = &snapshot{
			id:    id,
			value: val,
		}
	}

	return snaps, nil
}

// encode encodes a snapshots object into a slice of bytes.
func encode(snaps snapshots) ([]byte, error) {
	var buf bytes.Buffer
	var err error

	ids := []string{}
	for id := range snaps {
		ids = append(ids, string(id))
	}

	sort.Strings(ids)

	for _, id := range ids {
		s := snaps[snapshotID(id)]

		_, err = buf.WriteString(fmt.Sprintf("%s%s */\n", snapshotSeparator, string(s.id)))
		if err != nil {
			return nil, err
		}
		_, err = buf.WriteString(fmt.Sprintf("%s\n\n", s.value))
		if err != nil {
			return nil, err
		}
	}

	return bytes.TrimSpace(buf.Bytes()), nil
}

// loadSnapshots loads all snapshots in the current directory, can only
// be called once
func loadSnapshots() (err error) {
	snapshotsLoaded.Do(func() {
		err = reloadSnapshots()
	})
	return err
}

// reloadSnapshots overwrites allSnapshots internal
// variable with the designated snapshots file
func reloadSnapshots() error {
	dir, err := findOrCreateSnapshotDirectory()
	if err != nil {
		return err
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	paths := []string{}
	for _, file := range files {
		path := filepath.Join(dir, file.Name())
		if isSnapshot(path) {
			paths = append(paths, path)
		}
	}

	allSnapshots, err = parseSnapshotsFromPaths(paths)
	return err
}

// getSnapshot retrieves a snapshot by id.
func getSnapshot(id snapshotID) *snapshot {
	if err := loadSnapshots(); err != nil {
		panic(err)
	}
	return allSnapshots[id]
}

// createSnapshot creates a Snapshot.
func createSnapshot(id snapshotID, value string) (*snapshot, error) {
	return writeSnapshot(id, value, false)
}

// updateSnapshot creates a Snapshot.
func updateSnapshot(id snapshotID, value string) (*snapshot, error) {
	return writeSnapshot(id, value, true)
}

// writeSnapshot creates or updates a Snapshot.
func writeSnapshot(id snapshotID, value string, isUpdate bool) (*snapshot, error) {
	if !id.isValid() {
		return nil, errInvalidSnapshotID
	}

	if err := loadSnapshots(); err != nil {
		return nil, err
	}

	dir, err := findOrCreateSnapshotDirectory()
	if err != nil {
		return nil, err
	}

	pkg, err := getTestingPackage()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, fmt.Sprintf("%s%s", pkg, snapshotExt))

	s := &snapshot{
		id:        id,
		value:     value,
		path:      path,
		evaluated: isUpdate,
	}

	allSnapMutex.Lock()
	allSnapshots[id] = s
	allSnapMutex.Unlock()

	err = allSnapshots.save()
	if err != nil {
		return nil, err
	}

	return s, nil
}

func findOrCreateSnapshotDirectory() (string, error) {
	testingPath, err := getTestingPath()
	if err != nil {
		return "", errUnableToLocateTestPath
	}

	dir := filepath.Join(testingPath, SnapshotsDir)
	_, err = os.Stat(dir)
	if os.IsNotExist(err) {
		err = os.Mkdir(dir, os.ModePerm)
		if err != nil {
			return "", errUnableToCreateSnapshotDirectory
		}
	}

	return dir, nil
}

func parseSnapshotsFromPaths(paths []string) (snapshots, error) {
	var snaps = make(snapshots)
	var mutex = &sync.Mutex{}

	var wg sync.WaitGroup
	for i := range paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			file, err := os.Open(p)
			if err != nil {
				return
			}

			data, err := ioutil.ReadAll(file)
			if err != nil {
				return
			}

			s, err := decode(data)
			if err != nil {
				return
			}

			mutex.Lock()
			for id, snap := range s {
				snap.path = p
				snaps[id] = snap
			}
			mutex.Unlock()
		}(paths[i])
	}
	wg.Wait()

	return snaps, nil
}

func getTestingPath() (string, error) {
	return os.Getwd()
}

func getTestingPackage() (string, error) {
	dir, err := getTestingPath()
	if err != nil {
		return "", err
	}

	return filepath.Base(dir), nil
}
