package featuremanagement

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// resolveKeyOrName returns the resource whose key is keyOrName or, when no
// resource has that key, the one whose name it is (design decision 2). The
// key is tried first and costs one request; a name costs a listing, and only
// when the key misses. Resolution never dispatches on the argument's shape,
// since the key pattern also matches names and UUIDs: the server decides. It
// answers 404 for a well-formed key nobody has, and 400 for an argument that
// cannot be a key at all ("Checkout flows"), which can then only be a name.
//
// kind is the singular resource name used in errors ("project").
func resolveKeyOrName[T any](kind, keyOrName string, get func(key string) (T, error), list func() ([]T, error), keyOf, nameOf func(T) string) (T, error) {
	var zero T
	item, err := get(keyOrName)
	if err == nil {
		return item, nil
	}
	if !isKeyMiss(err) {
		return zero, err
	}

	items, listErr := list()
	if listErr != nil {
		// The name lookup is a fallback; the key's error is the one that
		// answers what the caller asked.
		return zero, err
	}
	var matches []T
	for _, it := range items {
		if nameOf(it) == keyOrName {
			matches = append(matches, it)
		}
	}
	switch len(matches) {
	case 0:
		return zero, fmt.Errorf("%s %q not found (no %s has that key or name)", kind, keyOrName, kind)
	case 1:
		return matches[0], nil
	default:
		keys := make([]string, len(matches))
		for i, m := range matches {
			keys[i] = keyOf(m)
		}
		sort.Strings(keys)
		return zero, fmt.Errorf("%d %ss are named %q; pass a key instead: %s", len(matches), kind, keyOrName, strings.Join(keys, ", "))
	}
}

// isKeyMiss reports whether a lookup by key failed because the argument is
// not an existing key: a 404, or the 400 the API returns when the argument
// does not match the key pattern.
func isKeyMiss(err error) bool {
	var apiErr *httpclient.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusBadRequest
}

// displayTime formats a timestamp for a table column, in UTC, or "" when
// unset.
func displayTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

// onOff renders a boolean setting for a table column.
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
