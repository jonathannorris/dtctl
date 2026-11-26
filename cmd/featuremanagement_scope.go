package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/config"
	"github.com/Dynatrace/dtctl/pkg/stability"
)

// fmProjectFlag is the persistent flag on every verb's fm group that names
// the project a command works in (design decision 1: the kubectl --namespace
// model). It is only registered on FM commands, so the short name cannot
// collide with anything else.
const fmProjectFlag = "project"

// fmProjectContextFlag is the set-context flag that stores a context's
// default project. It is prefixed because set-context's flags configure the
// whole environment.
const fmProjectContextFlag = "fm-project"

// fmSince is the release that ships the first Feature Management flag on a
// command outside the development-tier fm groups.
const fmSince = "0.48.0"

// addFMProjectFlag declares --project on an fm group, for every resource
// below it. The group's development tier covers the flag.
func addFMProjectFlag(group *cobra.Command) {
	group.PersistentFlags().String(fmProjectFlag, "", "Feature Management project key (default: the context's --fm-project)")
}

// addFMProjectContextFlag declares --fm-project on a set-context command.
//
// The command is stable and always registered, so the flag cannot borrow the
// fm groups' development tier. It is experimental, and hidden until Feature
// Management leaves the development tier, so the help of a stable command
// does not advertise a feature that is not registered by default.
func addFMProjectContextFlag(cmd *cobra.Command) {
	cmd.Flags().String(fmProjectContextFlag, "", "default Feature Management project for this context (\"\" clears it)")
	stability.MarkFlag(cmd, fmProjectContextFlag, stability.Experimental, fmSince)
	_ = cmd.Flags().MarkHidden(fmProjectContextFlag)
}

// resolveFMProject returns the project an FM command works in: --project,
// else the current context's default, else an error saying how to set one.
func resolveFMProject(cmd *cobra.Command, cfg *config.Config) (string, error) {
	if f := cmd.Flags().Lookup(fmProjectFlag); f != nil && f.Changed {
		p := strings.TrimSpace(f.Value.String())
		if p == "" {
			return "", fmt.Errorf("--project must not be empty")
		}
		return p, nil
	}
	if cfg != nil {
		if p := cfg.FMProject(); p != "" {
			return p, nil
		}
	}
	ctxName := "<context>"
	if cfg != nil && cfg.CurrentContext != "" {
		ctxName = cfg.CurrentContext
	}
	return "", fmt.Errorf("no Feature Management project set; pass --project or run 'dtctl config set-context %s --fm-project <project>'", ctxName)
}
