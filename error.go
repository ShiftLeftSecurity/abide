package abide

import (
	"errors"
)

var (
	errUnableToLocateTestPath          = errors.New("unable to locate test path")
	errUnableToCreateSnapshotDirectory = errors.New("unable to create snapshot directory")
	errInvalidSnapshotID               = errors.New("invalid snapshot id")
)
