package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/CCCCY-ci/ctxhop/internal/config"
	"github.com/CCCCY-ci/ctxhop/internal/desktopbundle"
)

type bundleOptions struct{ action, input, metadata, id, output string }

func init() {
	commands = append(commands, command{name: "bundle", summary: "transfer encrypted opaque Desktop archives", run: runBundle})
	commandSubcommands["bundle"] = []string{"put", "list", "get"}
	commandOptions["bundle put"] = []string{"--input", "--metadata", "--json"}
	commandOptions["bundle list"] = []string{"--json"}
	commandOptions["bundle get"] = []string{"--id", "--output", "--json"}
}

func runBundle(args []string) error {
	return runBundleWithStreams(args, os.Stdin, os.Stdout, os.Stderr)
}

func parseBundleOptions(args []string) (bundleOptions, error) {
	var options bundleOptions
	if len(args) == 0 {
		return options, errors.New("bundle: put, list, or get is required")
	}
	options.action = args[0]
	flags := flag.NewFlagSet("bundle", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "required machine-readable output")
	switch options.action {
	case "put":
		flags.StringVar(&options.input, "input", "", "opaque archive path")
		flags.StringVar(&options.metadata, "metadata", "", "whitelisted metadata JSON path")
	case "list":
	case "get":
		flags.StringVar(&options.id, "id", "", "opaque bundle ID")
		flags.StringVar(&options.output, "output", "", "new output archive path")
	default:
		return options, errors.New("bundle: unknown action")
	}
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || !*jsonOutput {
		return options, errors.New("bundle: invalid arguments; --json is required")
	}
	if options.action == "put" && (options.input == "" || options.metadata == "") {
		return options, errors.New("bundle put: --input and --metadata are required")
	}
	if options.action == "get" {
		if options.output == "" {
			return options, errors.New("bundle get: --output is required")
		}
		if err := desktopbundle.ValidateID(options.id); err != nil {
			return options, err
		}
	}
	return options, nil
}

func runBundleWithStreams(args []string, input io.Reader, output, prompt io.Writer) error {
	options, err := parseBundleOptions(args)
	if err != nil {
		return err
	}
	if input == nil || output == nil || prompt == nil {
		return errors.New("bundle: command streams are required")
	}
	var metadata desktopbundle.Metadata
	if options.action == "put" {
		metadata, err = desktopbundle.ReadMetadataFile(options.metadata)
		if err != nil {
			return err
		}
	}
	configDir, err := config.Dir()
	if err != nil {
		return err
	}
	c, err := config.Load(configDir)
	if err != nil {
		return err
	}
	if config.ValidateDeviceID(c.Device.ID) != nil {
		return errors.New("bundle: invalid configured device ID")
	}
	if options.action == "put" {
		if configuredDeviceMode(c) == config.DeviceModeDisabled {
			return errors.New("bundle put: device is disabled")
		}
	} else if err := devicePullError("bundle", c); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var access *domainAccess
	if options.action == "put" {
		access, err = openAuthorizedDomain(ctx, c, configDir, "bundle")
	} else {
		access, err = openDomainForRead(ctx, c, configDir, input, prompt, "bundle")
	}
	if err != nil {
		return err
	}
	defer access.close()
	var report any
	switch options.action {
	case "put":
		report, err = desktopbundle.Put(ctx, access.Store, access.Public, c.Device.ID, options.input, metadata)
	case "list":
		var bundles []desktopbundle.Info
		bundles, err = desktopbundle.List(ctx, access.Store, access.Identities, access.allowedDevices(), prompt)
		report = struct {
			Bundles []desktopbundle.Info `json:"bundles"`
		}{Bundles: bundles}
	case "get":
		report, err = desktopbundle.Get(ctx, access.Store, access.Identities, access.allowedDevices(), options.id, options.output)
	}
	if err != nil {
		return fmt.Errorf("bundle %s: %w", options.action, err)
	}
	return json.NewEncoder(output).Encode(report)
}
