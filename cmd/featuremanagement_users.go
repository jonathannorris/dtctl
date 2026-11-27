package cmd

import (
	"regexp"
	"strings"

	"github.com/Dynatrace/dtctl/pkg/client"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	resiam "github.com/Dynatrace/dtctl/pkg/resources/iam"
)

var fmUserIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// fmUserNames turns the user IDs Feature Management returns (creator, last
// modifier, owner, maintainers) into names for the describe text views. It is
// best effort: an ID it cannot resolve, because the token lacks the IAM read
// scope, the user is gone, or the value is not a user ID at all, prints as
// it came. Structured output (-o json|yaml, agent mode) never goes through
// it, so it keeps the IDs the API returned.
type fmUserNames struct {
	lookup func(id string) (*resiam.User, error)
	cache  map[string]string
}

func newFMUserNames(c *client.Client) *fmUserNames {
	h := resiam.NewHandler(c)
	return &fmUserNames{lookup: h.GetUser, cache: map[string]string{}}
}

// name is the display name for one ID: "Name Surname (<id prefix>)", the
// email when the user has no name, or the ID itself when it does not resolve.
func (u *fmUserNames) name(id string) string {
	if u == nil || !fmUserIDPattern.MatchString(id) {
		return id
	}
	if n, ok := u.cache[id]; ok {
		return n
	}
	n := id
	if user, err := u.lookup(id); err == nil && user != nil {
		label := strings.TrimSpace(user.Name + " " + user.Surname)
		if label == "" {
			label = user.Email
		}
		if label != "" {
			n = label + " (" + id[:8] + ")"
		}
	}
	u.cache[id] = n
	return n
}

func (u *fmUserNames) names(ids []string) []string {
	if len(ids) == 0 {
		return ids
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = u.name(id)
	}
	return out
}

func (u *fmUserNames) modificationInfo(mi *featuremanagement.ModificationInfo) {
	mi.CreatedBy = u.name(mi.CreatedBy)
	mi.LastModifiedBy = u.name(mi.LastModifiedBy)
}
