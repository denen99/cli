package configure

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fastly/cli/pkg/api"
	"github.com/fastly/cli/pkg/common"
	"github.com/fastly/cli/pkg/config"
	"github.com/fastly/cli/pkg/text"
)

// APIClientFactory allows the configure command to regenerate the global Fastly
// API client when a new token is provided, in order to validate that token.
// It's a redeclaration of the app.APIClientFactory to avoid an import loop.
type APIClientFactory func(token, endpoint string) (api.Interface, error)

// RootCommand is the parent command for all subcommands in this package.
// It should be installed under the primary root command.
type RootCommand struct {
	common.Base
	configFilePath string
	clientFactory  APIClientFactory
}

// NewRootCommand returns a new command registered in the parent.
func NewRootCommand(parent common.Registerer, configFilePath string, cf APIClientFactory, globals *config.Data) *RootCommand {
	var c RootCommand
	c.Globals = globals
	c.CmdClause = parent.Command("configure", "Configure the Fastly CLI")
	c.configFilePath = configFilePath
	c.clientFactory = cf
	return &c
}

// Exec implements the command interface.
func (c *RootCommand) Exec(in io.Reader, out io.Writer) error {
	// Get the endpoint provided by the user, if it was explicitly provided. If
	// it wasn't provided use default.
	endpoint, source := c.Globals.Endpoint()
	switch source { // TODO(pb): this can be duplicate output if --verbose is passed
	case config.SourceFlag:
		text.Output(out, "Fastly API endpoint (via --endpoint): %s", endpoint)
	case config.SourceEnvironment:
		text.Output(out, "Fastly API endpoint (via %s): %s", config.EnvVarEndpoint, endpoint)
	}

	// Get the token provided by the user, if it was explicitly provided. If it
	// wasn't provided, or if it only exists in the config file, take it
	// interactively.
	token, source := c.Globals.Token()
	switch source { // TODO(pb): this can be duplicate output if --verbose is passed
	case config.SourceFlag:
		text.Output(out, "Fastly API token (via --token): %s", token)
	case config.SourceEnvironment:
		text.Output(out, "Fastly API token (via %s): %s", config.EnvVarToken, token)
	default:
		text.Output(out, `
			An API token is used to authenticate requests to the Fastly API.
			To create a token, visit https://manage.fastly.com/account/personal/tokens
		`)
		text.Break(out)
		var err error
		token, err = text.InputSecure(out, "Fastly API token: ", in, validateTokenNotEmpty)
		if err != nil {
			return err
		}
	}
	text.Break(out)

	text.Output(out, "Validating token...")

	client, err := c.clientFactory(token, endpoint)
	if err != nil {
		return fmt.Errorf("error regenerating Fastly API client: %w", err)
	}
	if _, err := client.GetTokenSelf(); err != nil {
		return fmt.Errorf("error validating token: %w", err)
	}

	// Set everything in the File struct based on provided user input.
	c.Globals.File.Token = token
	c.Globals.File.Endpoint = endpoint

	// Make sure the config file directory exists.
	dir := filepath.Dir(c.configFilePath)
	fi, err := os.Stat(dir)
	switch {
	case err == nil && fi.IsDir():
		// good
	case err == nil && !fi.IsDir():
		return fmt.Errorf("config file path %s isn't a directory", dir)
	case err != nil && os.IsNotExist(err):
		if err := os.MkdirAll(dir, config.DirectoryPermissions); err != nil {
			return fmt.Errorf("error creating config file directory: %w", err)
		}
	}

	// Write the file data to disk.
	if err := c.Globals.File.Write(c.configFilePath); err != nil {
		return fmt.Errorf("error saving config file: %w", err)
	}

	// Escape any spaces in filepath before output.
	filePath := strings.ReplaceAll(c.configFilePath, " ", `\ `)

	text.Success(out, "Configured the Fastly CLI")
	fmt.Fprintf(out, "\nYou can find your configuration file at\n\n  %s\n\n", filePath)
	return nil
}

func validateTokenNotEmpty(s string) error {
	if s == "" {
		return ErrEmptyToken
	}
	return nil
}

// ErrEmptyToken is returned when a user tries to supply an emtpy string as a
// token in the configure command.
var ErrEmptyToken = errors.New("token cannot be empty")
