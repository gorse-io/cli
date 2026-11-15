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
	"strings"
	"syscall"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

// Keep keyring identifiers stable so saved applications remain available after the rename.
const (
	keyringService         = "gorse-cli"
	keyringCurrentKey      = "current"
	keyringApplicationsKey = "applications"
)

var (
	applicationNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	applications           = NewApplications(keyringService)
)

type Application struct {
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

func (c Applications) List() ([]string, error) {
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
	names, err := c.List()
	if err != nil {
		return err
	}
	if !lo.Contains(names, name) {
		names = append(names, name)
	}
	return c.saveNames(names)
}

func (c Applications) removeName(name string) ([]string, error) {
	names, err := c.List()
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

func (c Applications) Current() (string, error) {
	name, err := keyring.Get(c.keyringService, keyringCurrentKey)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return name, err
}

func (c Applications) SetCurrent(name string) error {
	return keyring.Set(c.keyringService, keyringCurrentKey, name)
}

func (c Applications) ClearCurrent() error {
	if err := keyring.Delete(c.keyringService, keyringCurrentKey); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

func (c Applications) Get(name string) (Application, error) {
	if !applicationNamePattern.MatchString(name) {
		return Application{}, fmt.Errorf("application name %q must match %q", name, applicationNamePattern.String())
	}
	endpoint, endpointErr := keyring.Get(c.keyringService, c.applicationEndpointKey(name))
	apiKey, apiKeyErr := keyring.Get(c.keyringService, c.applicationAPIKeyKey(name))
	if errors.Is(endpointErr, keyring.ErrNotFound) || errors.Is(apiKeyErr, keyring.ErrNotFound) {
		return Application{}, fmt.Errorf("application %q not found", name)
	}
	if endpointErr != nil {
		return Application{}, endpointErr
	}
	if apiKeyErr != nil {
		return Application{}, apiKeyErr
	}
	return Application{Name: name, Endpoint: endpoint, APIKey: apiKey}, nil
}

func (c Applications) Upsert(name, endpoint, apiKey string) error {
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
	return c.SetCurrent(name)
}

func (c Applications) Delete(name string) error {
	if _, err := c.Get(name); err != nil {
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
	current, err := c.Current()
	if err != nil {
		return err
	}
	if current != name {
		return nil
	}
	if len(names) == 0 {
		return c.ClearCurrent()
	}
	return c.SetCurrent(names[0])
}

// getEndpointAndKey returns the base URL and API key from flags, application, or environment variables.
func getEndpointAndKey(cmd *cobra.Command) (endpoint, apiKey string) {
	endpoint = getFlagOrEnv(cmd, "endpoint", "GORSE_ADMIN_ENDPOINT")
	apiKey = getFlagOrEnv(cmd, "api-key", "GORSE_ADMIN_API_KEY")

	if endpoint == "" || apiKey == "" {
		applicationName := lo.Must(cmd.Flags().GetString("application"))
		if applicationName != "" {
			app, err := applications.Get(applicationName)
			if err != nil {
				output := cmd.ErrOrStderr()
				fmt.Fprintf(output, "Error: application %q was not found.\n", applicationName)
				fmt.Fprintln(output)
				fmt.Fprintln(output, "List available applications:")
				fmt.Fprintln(output, "  gorse-cli application list")
				fmt.Fprintln(output, "Create it:")
				fmt.Fprintln(output, "  gorse-cli application add "+applicationName+" --endpoint http://localhost:8088")
				os.Exit(1)
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
		applicationName, err := applications.Current()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to read the current application from the system keyring: %v\n", err)
			os.Exit(1)
		}
		if applicationName != "" {
			app, err := applications.Get(applicationName)
			if err != nil {
				output := cmd.ErrOrStderr()
				fmt.Fprintf(output, "Error: current application %q is invalid or incomplete.\n", applicationName)
				fmt.Fprintln(output)
				fmt.Fprintln(output, "Choose another application:")
				fmt.Fprintln(output, "  gorse-cli application use <name>")
				fmt.Fprintln(output, "Or recreate it:")
				fmt.Fprintln(output, "  gorse-cli application add "+applicationName+" --endpoint http://localhost:8088")
				os.Exit(1)
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

var ApplicationCmd = &cobra.Command{
	Use:   "application",
	Short: "Manage applications",
}

var applicationAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add or update a application",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		endpoint := getFlagOrEnv(cmd, "endpoint", "GORSE_ADMIN_ENDPOINT")
		apiKey := getFlagOrEnv(cmd, "api-key", "GORSE_ADMIN_API_KEY")

		if apiKey == "" {
			var err error
			apiKey, err = readSecret("Gorse admin API key: ")
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to read API key: %v\n", err)
				os.Exit(1)
			}
		}
		if err := applications.Upsert(name, endpoint, apiKey); err != nil {
			output := cmd.ErrOrStderr()
			fmt.Fprintf(output, "Error: failed to save application %q: %v\n", name, err)
			fmt.Fprintln(output)
			fmt.Fprintln(output, "Example:")
			fmt.Fprintln(output, "  gorse-cli application add "+name+" --endpoint http://localhost:8088")
			os.Exit(1)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Application %q saved and selected.\n", name)
	},
}

var applicationListCmd = &cobra.Command{
	Use:   "list",
	Short: "List applications",
	Run: func(cmd *cobra.Command, args []string) {
		names, err := applications.List()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to read applications from the system keyring: %v\n", err)
			os.Exit(1)
		}
		if len(names) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No applications configured.")
			return
		}
		current, err := applications.Current()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to read the current application from the system keyring: %v\n", err)
			os.Exit(1)
		}
		rows := make([][]string, 0, len(names))
		for _, name := range names {
			app, err := applications.Get(name)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: application %q is invalid or incomplete: %v\n", name, err)
				os.Exit(1)
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
	Short: "Switch the current application",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if _, err := applications.Get(name); err != nil {
			output := cmd.ErrOrStderr()
			fmt.Fprintf(output, "Error: application %q was not found.\n", name)
			fmt.Fprintln(output)
			fmt.Fprintln(output, "List available applications:")
			fmt.Fprintln(output, "  gorse-cli application list")
			os.Exit(1)
		}
		if err := applications.SetCurrent(name); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to save the current application in the system keyring: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Switched to application %q.\n", name)
	},
}

var applicationDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a application",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if err := applications.Delete(name); err != nil {
			output := cmd.ErrOrStderr()
			fmt.Fprintf(output, "Error: application %q was not found.\n", name)
			fmt.Fprintln(output)
			fmt.Fprintln(output, "List available applications:")
			fmt.Fprintln(output, "  gorse-cli application list")
			os.Exit(1)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Application %q deleted.\n", name)
	},
}

var applicationCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the current application",
	Run: func(cmd *cobra.Command, args []string) {
		name, err := applications.Current()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to read the current application from the system keyring: %v\n", err)
			os.Exit(1)
		}
		if name == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "No current application.")
			return
		}
		app, err := applications.Get(name)
		if err != nil {
			output := cmd.ErrOrStderr()
			fmt.Fprintf(output, "Error: current application %q is invalid or incomplete.\n", name)
			fmt.Fprintln(output)
			fmt.Fprintln(output, "Choose another application:")
			fmt.Fprintln(output, "  gorse-cli application use <name>")
			fmt.Fprintln(output, "Or add it:")
			fmt.Fprintln(output, "  gorse-cli application add "+name+" --endpoint http://localhost:8088")
			os.Exit(1)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Name:\t\t%s\nEndpoint:\t%s\n", app.Name, app.Endpoint)
	},
}

func init() {
	ApplicationCmd.AddCommand(applicationAddCmd)
	ApplicationCmd.AddCommand(applicationListCmd)
	ApplicationCmd.AddCommand(applicationUseCmd)
	ApplicationCmd.AddCommand(applicationDeleteCmd)
	ApplicationCmd.AddCommand(applicationCurrentCmd)
	applicationAddCmd.Flags().String("endpoint", "", "Gorse base URL (default: GORSE_ADMIN_ENDPOINT)")
	applicationAddCmd.Flags().String("api-key", "", "Gorse admin API key (default: GORSE_ADMIN_API_KEY; prefer prompt to avoid shell history)")
}
