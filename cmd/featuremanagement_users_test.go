package cmd

import (
	"errors"
	"testing"

	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	resiam "github.com/Dynatrace/dtctl/pkg/resources/iam"
)

func TestFMUserNames(t *testing.T) {
	const (
		named   = "00000000-0000-4000-8000-0000000000a1"
		mailed  = "00000000-0000-4000-8000-0000000000a2"
		unknown = "00000000-0000-4000-8000-0000000000a3"
	)
	calls := 0
	u := &fmUserNames{cache: map[string]string{}, lookup: func(id string) (*resiam.User, error) {
		calls++
		switch id {
		case named:
			return &resiam.User{UID: id, Name: "Ada", Surname: "Lovelace", Email: "ada@example.invalid"}, nil
		case mailed:
			return &resiam.User{UID: id, Email: "bob@example.invalid"}, nil
		}
		return nil, errors.New("403 forbidden")
	}}

	mi := featuremanagement.ModificationInfo{CreatedBy: named, LastModifiedBy: named}
	u.modificationInfo(&mi)
	if mi.CreatedBy != "Ada Lovelace (00000000)" || mi.LastModifiedBy != mi.CreatedBy || calls != 1 {
		t.Errorf("modification info = %+v after %d lookups", mi, calls)
	}
	got := u.names([]string{mailed, unknown, "checkout-team"})
	if got[0] != "bob@example.invalid (00000000)" || got[1] != unknown || got[2] != "checkout-team" {
		t.Errorf("names = %q", got)
	}
	if calls != 3 {
		t.Errorf("a value that is not a user ID was looked up (%d lookups)", calls)
	}
	if (*fmUserNames)(nil).name(named) != named {
		t.Error("a nil resolver must leave IDs alone")
	}
}
