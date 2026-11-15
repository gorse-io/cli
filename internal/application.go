// Copyright 2026 gorse Project Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

// Keep keyring identifiers stable so saved applications remain available after the rename.
const (
	keyringService               = "gorse-cli"
	keyringCurrentApplicationKey = "current-context"
	keyringApplicationsKey       = "contexts"
)

var (
	applicationNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	applications           = NewApplications(keyringService)
)

type cliApplication struct {
	Name     string
	Endpoint string
	APIKey   string
}

type Applications struct {
	keyringService string
}

func NewApplications(keyringService string) Applications {
	return Applications{keyringService: keyringService}
}

func (c Applications) applicationEndpointKey(name string) string {
	return "context:" + name + ":admin-endpoint"
}

func (c Applications) applicationAPIKeyKey(name string) string {
	return "context:" + name + ":admin-api-key"
}

func (c Applications) Names() ([]string, error) {
	raw, err := keyring.Get(c.keyringService, keyringApplicationsKey)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	if err = json.Unmarshal([]byte(raw), &names); err != nil {
		return nil, err
	}
	names = lo.Uniq(names)
	sort.Strings(names)
	return names, nil
}

func (c Applications) saveNames(names []string) error {
	names = lo.Uniq(names)
	sort.Strings(names)
	raw, err := json.Marshal(names)
	if err != nil {
		return err
	}
	return keyring.Set(c.keyringService, keyringApplicationsKey, string(raw))
}

func (c Applications) addName(name string) error {
	names, err := c.Names()
	if err != nil {
		return err
	}
	if !lo.Contains(names, name) {
		names = append(names, name)
	}
	return c.saveNames(names)
}

func (c Applications) removeName(name string) ([]string, error) {
	names, err := c.Names()
	if err != nil {
		return nil, err
	}
	names = lo.Filter(names, func(candidate string, _ int) bool {
		return candidate != name
	})
	if err = c.saveNames(names); err != nil {
		return nil, err
	}
	return names, nil
}

func (c Applications) CurrentName() (string, error) {
	name, err := keyring.Get(c.keyringService, keyringCurrentApplicationKey)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return name, err
}

func (c Applications) SetCurrentName(name string) error {
	return keyring.Set(c.keyringService, keyringCurrentApplicationKey, name)
}

func (c Applications) ClearCurrentName() error {
	if err := keyring.Delete(c.keyringService, keyringCurrentApplicationKey); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

func (c Applications) Load(name string) (cliApplication, error) {
	if !applicationNamePattern.MatchString(name) {
		return cliApplication{}, fmt.Errorf("application name %q must match %q", name, applicationNamePattern.String())
	}
	endpoint, endpointErr := keyring.Get(c.keyringService, c.applicationEndpointKey(name))
	apiKey, apiKeyErr := keyring.Get(c.keyringService, c.applicationAPIKeyKey(name))
	if errors.Is(endpointErr, keyring.ErrNotFound) || errors.Is(apiKeyErr, keyring.ErrNotFound) {
		return cliApplication{}, fmt.Errorf("application %q not found", name)
	}
	if endpointErr != nil {
		return cliApplication{}, endpointErr
	}
	if apiKeyErr != nil {
		return cliApplication{}, apiKeyErr
	}
	return cliApplication{Name: name, Endpoint: endpoint, APIKey: apiKey}, nil
}

func (c Applications) Save(name, endpoint, apiKey string) error {
	if !applicationNamePattern.MatchString(name) {
		return fmt.Errorf("application name %q must match %q", name, applicationNamePattern.String())
	}
	if endpoint == "" {
		return fmt.Errorf("GORSE_ADMIN_ENDPOINT or --endpoint is required")
	}
	if apiKey == "" {
		return fmt.Errorf("GORSE_ADMIN_API_KEY or --api-key is required")
	}
	if err := keyring.Set(c.keyringService, c.applicationEndpointKey(name), endpoint); err != nil {
		return err
	}
	if err := keyring.Set(c.keyringService, c.applicationAPIKeyKey(name), apiKey); err != nil {
		return err
	}
	if err := c.addName(name); err != nil {
		return err
	}
	return c.SetCurrentName(name)
}

func (c Applications) Delete(name string) error {
	if _, err := c.Load(name); err != nil {
		return err
	}
	if err := keyring.Delete(c.keyringService, c.applicationEndpointKey(name)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	if err := keyring.Delete(c.keyringService, c.applicationAPIKeyKey(name)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	names, err := c.removeName(name)
	if err != nil {
		return err
	}
	current, err := c.CurrentName()
	if err != nil {
		return err
	}
	if current != name {
		return nil
	}
	if len(names) == 0 {
		return c.ClearCurrentName()
	}
	return c.SetCurrentName(names[0])
}

// getEndpointAndKey returns the base URL and API key from flags, application, or environment variables.
func getEndpointAndKey(cmd *cobra.Command) (endpoint, apiKey string) {
	endpoint = getFlagOrEnv(cmd, "endpoint", "GORSE_ADMIN_ENDPOINT")
	apiKey = getFlagOrEnv(cmd, "api-key", "GORSE_ADMIN_API_KEY")

	if endpoint == "" || apiKey == "" {
		applicationName := lo.Must(cmd.Flags().GetString("application"))
		if applicationName != "" {
			app, err := applications.Load(applicationName)
			if err != nil {
				fatal(cmd,
					fmt.Sprintf("application %q was not found.", applicationName),
					"List available applications:",
					"  gorse-cli application list",
					"Create it:",
					"  gorse-cli application add "+applicationName+" --endpoint http://localhost:8088",
				)
			}
			if endpoint == "" {
				endpoint = app.Endpoint
			}
			if apiKey == "" {
				apiKey = app.APIKey
			}
		}
	}

	if endpoint == "" || apiKey == "" {
		applicationName, err := applications.CurrentName()
		if err != nil {
			fatal(cmd, "failed to read the current application from the system keyring: "+err.Error())
		}
		if applicationName != "" {
			app, err := applications.Load(applicationName)
			if err != nil {
				fatal(cmd,
					fmt.Sprintf("current application %q is invalid or incomplete.", applicationName),
					"Choose another application:",
					"  gorse-cli application use <name>",
					"Or recreate it:",
					"  gorse-cli application add "+applicationName+" --endpoint http://localhost:8088",
				)
			}
			if endpoint == "" {
				endpoint = app.Endpoint
			}
			if apiKey == "" {
				apiKey = app.APIKey
			}
		}
	}
	return
}

func getFlagOrEnv(cmd *cobra.Command, flagName, envName string) string {
	value := lo.Must(cmd.Flags().GetString(flagName))
	if value != "" {
		return value
	}
	return os.Getenv(envName)
}

func readSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(syscall.Stdin)) {
		secret, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(string(secret)), err
	}
	reader := bufio.NewReader(os.Stdin)
	secret, err := reader.ReadString('\n')
	return strings.TrimSpace(secret), err
}

func fatal(cmd *cobra.Command, message string, suggestions ...string) {
	output := cmd.ErrOrStderr()
	fmt.Fprintf(output, "Error: %s\n", message)
	if len(suggestions) > 0 {
		fmt.Fprintln(output)
		for _, suggestion := range suggestions {
			fmt.Fprintln(output, suggestion)
		}
	}
	os.Exit(1)
}

func fatalErr(cmd *cobra.Command, message string, err error) {
	if err != nil {
		message += ": " + err.Error()
	}
	fatal(cmd, message)
}

var applicationCmd = &cobra.Command{
	Use:   "application",
	Short: "Manage Gorse CLI applications",
}

var applicationAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add or update a Gorse CLI application",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		endpoint := getFlagOrEnv(cmd, "endpoint", "GORSE_ADMIN_ENDPOINT")
		apiKey := getFlagOrEnv(cmd, "api-key", "GORSE_ADMIN_API_KEY")

		if apiKey == "" {
			var err error
			apiKey, err = readSecret("Gorse admin API key: ")
			if err != nil {
				fatal(cmd, "failed to read API key: "+err.Error())
			}
		}
		if err := applications.Save(name, endpoint, apiKey); err != nil {
			fatal(cmd,
				"failed to save application "+strconv.Quote(name)+": "+err.Error(),
				"Example:",
				"  gorse-cli application add "+name+" --endpoint http://localhost:8088",
			)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Application %q saved and selected.\n", name)
	},
}

var applicationListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Gorse CLI applications",
	Run: func(cmd *cobra.Command, args []string) {
		names, err := applications.Names()
		if err != nil {
			fatal(cmd, "failed to read applications from the system keyring: "+err.Error())
		}
		if len(names) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No applications configured.")
			return
		}
		current, err := applications.CurrentName()
		if err != nil {
			fatal(cmd, "failed to read the current application from the system keyring: "+err.Error())
		}
		rows := make([][]string, 0, len(names))
		for _, name := range names {
			app, err := applications.Load(name)
			if err != nil {
				fatal(cmd, "application "+strconv.Quote(name)+" is invalid or incomplete: "+err.Error())
			}
			currentMarker := ""
			if name == current {
				currentMarker = "*"
			}
			rows = append(rows, []string{currentMarker, name, app.Endpoint})
		}
		printTable(cmd.OutOrStdout(), []string{"Current", "Name", "Endpoint"}, rows)
	},
}

var applicationUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Switch the current Gorse CLI application",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if _, err := applications.Load(name); err != nil {
			fatal(cmd,
				"application "+strconv.Quote(name)+" was not found.",
				"List available applications:",
				"  gorse-cli application list",
			)
		}
		if err := applications.SetCurrentName(name); err != nil {
			fatal(cmd, "failed to save the current application in the system keyring: "+err.Error())
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Switched to application %q.\n", name)
	},
}

var applicationDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a Gorse CLI application",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if err := applications.Delete(name); err != nil {
			fatal(cmd,
				"application "+strconv.Quote(name)+" was not found.",
				"List available applications:",
				"  gorse-cli application list",
			)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Application %q deleted.\n", name)
	},
}

var applicationCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the current Gorse CLI application",
	Run: func(cmd *cobra.Command, args []string) {
		name, err := applications.CurrentName()
		if err != nil {
			fatal(cmd, "failed to read the current application from the system keyring: "+err.Error())
		}
		if name == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "No current application.")
			return
		}
		app, err := applications.Load(name)
		if err != nil {
			fatal(cmd,
				"current application "+strconv.Quote(name)+" is invalid or incomplete.",
				"Choose another application:",
				"  gorse-cli application use <name>",
				"Or recreate it:",
				"  gorse-cli application add "+name+" --endpoint http://localhost:8088",
			)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Name:\t\t%s\nEndpoint:\t%s\n", app.Name, app.Endpoint)
	},
}
