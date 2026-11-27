package featuremanagement

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/Dynatrace/dtctl/pkg/util/format"
	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// WriteOptions are the write controls the update commands share.
type WriteOptions struct {
	// Version, when set, is sent as the optimistic-locking version in place
	// of the one just read, so the write fails with 409 unless the resource
	// is still at it.
	Version string
}

// version returns the optimistic-locking version a write sends: the caller's
// when set, else the one just read.
func (o WriteOptions) version(read int) string {
	if o.Version != "" {
		return o.Version
	}
	return strconv.Itoa(read)
}

// conflictError explains a 409 on a read-modify-write: the resource changed
// between the read and the write. The API error stays wrapped, so the agent
// envelope still reports the conflict code.
func conflictError(err error, what string) error {
	if errors.Is(err, httpclient.ErrConflict) {
		return fmt.Errorf("%s changed since it was read; re-run to retry: %w", what, err)
	}
	return err
}

// ResourceFile is a create body read from a YAML or JSON file.
type ResourceFile struct {
	Kind string
	// ReadOnly are members a read returns that a create does not take; a
	// file exported with get -o yaml carries them, so they are dropped.
	ReadOnly []string
	// Parents are members naming the resource's parents, with the value the
	// command targets. A file naming a different parent is refused; deleting
	// the member retargets the file.
	Parents map[string]string
}

// Decode parses data as YAML or JSON into v, after checking the file's
// optional `kind` and parent keys and dropping the read-only members.
func (f ResourceFile) Decode(data []byte, v any) error {
	jsonData, err := format.YAMLToJSON(data)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(jsonData, &doc); err != nil {
		return fmt.Errorf("the file must hold one object: %w", err)
	}
	if k, ok := doc["kind"]; ok {
		if k != f.Kind {
			return fmt.Errorf("the file is a %v, not a %s", k, f.Kind)
		}
		delete(doc, "kind")
	}
	for member, want := range f.Parents {
		if got, ok := doc[member]; ok && got != want {
			return fmt.Errorf("the file's %s is %v, but the command targets %s; delete %s from the file to use it here", member, got, want, member)
		}
		delete(doc, member)
	}
	for _, member := range f.ReadOnly {
		delete(doc, member)
	}
	clean, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return json.Unmarshal(clean, v)
}
