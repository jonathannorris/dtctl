package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	"github.com/Dynatrace/dtctl/pkg/safety"
)

var fmContextAttributeTypes = []string{"STRING", "NUMBER", "BOOLEAN", "SEM_VER", "DATE_TIME"}

func newCreateFMContextAttributeCmd() *cobra.Command {
	var file, name, description, attrType, contextKey, schema string
	c := &cobra.Command{
		Use:     "context-attribute <attribute-key> | -f <file>",
		Aliases: []string{"context-attributes"},
		Short:   "Create a context attribute",
		Long: `Create a context attribute: a typed value from the evaluation context that
targeting conditions can test, such as a user's country or an app version.

--type is the value type conditions compare it as, and --context-key the
name applications set it under in the evaluation context (default: the
attribute key). Neither can change after the attribute is created.

-f reads the API's ContextAttributeCreate fields from YAML or JSON, so an
attribute exported with 'dtctl get fm context-attribute <key> -o yaml'
creates a copy. The file's projectKey, if present, must be the target
project.

Examples:
  dtctl create fm context-attribute country --name Country --type STRING
  dtctl create fm context-attribute app-version --name "App version" --type SEM_VER --context-key appVersion
  dtctl create fm context-attribute -f attribute.yaml
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			if file != "" && changedAny(cmd, "name", "description", "type", "context-key", "schema") {
				return fmt.Errorf("-f cannot be combined with the field flags; put the fields in the file")
			}
			var data []byte
			if file != "" {
				var err error
				if data, err = readFileFlag(ctx, "file", file); err != nil {
					return err
				}
			} else {
				if len(args) == 0 {
					return fmt.Errorf("pass a context attribute key, or -f <file>")
				}
				if name == "" || attrType == "" {
					return fmt.Errorf("--name and --type are required (or use -f <file>)")
				}
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			var body featuremanagement.ContextAttributeCreate
			if file != "" {
				if err := featuremanagement.ContextAttributeFile(project).Decode(data, &body); err != nil {
					return fmt.Errorf("%s: %w", sourceName(file), err)
				}
				if len(args) > 0 {
					if body.Key != "" && body.Key != args[0] {
						return fmt.Errorf("the file creates context attribute %q, not %q", body.Key, args[0])
					}
					body.Key = args[0]
				}
				if body.Key == "" {
					return fmt.Errorf("the file has no key; add one or pass it as an argument")
				}
				if body.ContextKey == "" {
					body.ContextKey = body.Key
				}
			} else {
				t, err := fmEnum("type", attrType, fmContextAttributeTypes)
				if err != nil {
					return err
				}
				if contextKey == "" {
					contextKey = args[0]
				}
				body = featuremanagement.ContextAttributeCreate{
					Key: args[0], Name: name, Description: description, Schema: schema,
					Type: featuremanagement.ContextAttributeType(t), ContextKey: contextKey,
				}
			}
			res, err := h.CreateContextAttribute(project, body)
			if err != nil {
				return err
			}
			return printFMWrite(ctx, printer, "create", "fm/context-attribute",
				fmt.Sprintf("Created context attribute %s in project %s", res.Attribute.Key, project), true, nil, res.Attribute)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVarP(&file, "file", "f", "", "file containing the context attribute (YAML or JSON)")
	c.Flags().StringVar(&name, "name", "", "display name (required unless -f is given)")
	c.Flags().StringVar(&attrType, "type", "", "STRING|NUMBER|BOOLEAN|SEM_VER|DATE_TIME (required unless -f is given)")
	c.Flags().StringVar(&contextKey, "context-key", "", "name of the value in the evaluation context (default: the attribute key)")
	c.Flags().StringVar(&description, "description", "", "context attribute description")
	c.Flags().StringVar(&schema, "schema", "", "schema of the attribute's values, as the API stores it (default none)")
	markFileFlag(c, "file", fileValuePath)
	_ = c.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(fmContextAttributeTypes, cobra.ShellCompDirectiveNoFileComp))
	return c
}

func newUpdateFMContextAttributeCmd() *cobra.Command {
	var name, description, schema, version string
	c := &cobra.Command{
		Use:     "context-attribute <attribute-key-or-name>",
		Aliases: []string{"context-attributes"},
		Short:   "Change a context attribute's name, description or schema",
		Long: `Change a context attribute's name, description or schema, with the version it
read; a change that leaves the attribute as it was sends nothing. Its type and
context key cannot change: create a new attribute instead.

Examples:
  dtctl update fm context-attribute country --description "ISO 3166 country code"
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			if !changedAny(cmd, "name", "description", "schema") {
				return fmt.Errorf("nothing to update: pass --name, --description or --schema")
			}
			var change featuremanagement.ContextAttributeChange
			if cmd.Flags().Changed("name") {
				change.Name = &name
			}
			if cmd.Flags().Changed("description") {
				change.Description = &description
			}
			if cmd.Flags().Changed("schema") {
				change.Schema = &schema
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			res, err := h.UpdateContextAttribute(project, args[0], change, featuremanagement.WriteOptions{Version: version})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("Updated context attribute %s", res.Attribute.Key)
			if !res.Changed {
				msg = fmt.Sprintf("Nothing to change: context attribute %s already is as requested", res.Attribute.Key)
			}
			return printFMWrite(ctx, printer, "update", "fm/context-attribute", msg, res.Changed, nil, res.Attribute)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVar(&name, "name", "", "new display name")
	c.Flags().StringVar(&description, "description", "", "new description")
	c.Flags().StringVar(&schema, "schema", "", "new schema of the attribute's values")
	c.Flags().StringVar(&version, "version", "", fmVersionHelp)
	return c
}

func newDeleteFMContextAttributeCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:     "context-attribute <attribute-key-or-name>",
		Aliases: []string{"context-attributes"},
		Short:   "Delete a context attribute",
		Long: `Delete a context attribute. One a targeting rule still references is refused,
naming the features whose rules do. It asks for confirmation unless -y is
given; without a terminal to ask on, it needs -y.

Examples:
  dtctl delete fm context-attribute country
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			ok, err := confirmFMDelete(ctx, yes, fmt.Sprintf("context attribute %s", args[0]))
			if err != nil || !ok {
				return err
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationDelete)
			if err != nil {
				return err
			}
			key, err := h.DeleteContextAttribute(project, args[0])
			if err != nil {
				return err
			}
			return printFMDeleted(ctx, printer, "fm/context-attribute", fmt.Sprintf("Deleted context attribute %s from project %s", key, project),
				map[string]string{"projectKey": project, "key": key})
		},
	}
	addFMYesFlag(c, &yes)
	return c
}
