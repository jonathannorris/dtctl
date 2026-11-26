package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/config"
)

func fmScopeTestCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	group := newFMGroupCmd("test")
	leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
	group.AddCommand(leaf)
	if err := group.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	if err := leaf.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	return leaf
}

func fmConfig(project string) *config.Config {
	cfg := config.NewConfig()
	opts := &config.ContextOptions{}
	if project != "" {
		opts.FMProject = &project
	}
	cfg.SetContextWithOptions("dev", "https://dev.example.invalid", "dev-token", opts)
	cfg.CurrentContext = "dev"
	return cfg
}

func TestResolveFMProject(t *testing.T) {
	cases := []struct {
		name, ctxDefault string
		args             []string
		want, wantErr    string
	}{
		{name: "flag wins over the context", ctxDefault: "payments", args: []string{"--project", "checkout"}, want: "checkout"},
		{name: "context default", ctxDefault: "payments", want: "payments"},
		{name: "neither", wantErr: "no Feature Management project set; pass --project or run 'dtctl config set-context dev --fm-project <project>'"},
		{name: "explicitly empty flag", ctxDefault: "payments", args: []string{"--project", " "}, wantErr: "--project must not be empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveFMProject(fmScopeTestCmd(t, tc.args...), fmConfig(tc.ctxDefault))
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// set-context --fm-project writes the namespaced block, and "" clears it.
func TestSetContextFMProject(t *testing.T) {
	projectDir, _ := chdirWithLocalConfig(t)
	localPath := filepath.Join(projectDir, config.LocalConfigName)

	project := "checkout"
	if err := setContext(context.Background(), "my-environment", contextSettings{fmProject: &project}); err != nil {
		t.Fatalf("setContext(--fm-project): %v", err)
	}
	data, _ := os.ReadFile(localPath)
	if !strings.Contains(string(data), "feature-management:\n") || !strings.Contains(string(data), "project: checkout") {
		t.Fatalf("config lacks the feature-management block:\n%s", data)
	}

	empty := ""
	if err := setContext(context.Background(), "my-environment", contextSettings{fmProject: &empty}); err != nil {
		t.Fatalf("setContext(--fm-project \"\"): %v", err)
	}
	data, _ = os.ReadFile(localPath)
	if strings.Contains(string(data), "feature-management") {
		t.Fatalf("clearing the default left the block behind:\n%s", data)
	}
}

// The flag is on both spellings of set-context, and hidden while Feature
// Management is a development feature.
func TestFMProjectContextFlagIsHidden(t *testing.T) {
	for _, c := range []*cobra.Command{newConfigSetContextCmd(), ctxSetCmd} {
		f := c.Flags().Lookup(fmProjectContextFlag)
		if f == nil {
			t.Fatalf("%s has no --%s", c.CommandPath(), fmProjectContextFlag)
		}
		if !f.Hidden {
			t.Errorf("%s --%s is not hidden", c.CommandPath(), fmProjectContextFlag)
		}
	}
}
