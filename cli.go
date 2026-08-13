package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

const (
	exitSuccess = 0
	exitRuntime = 1
	exitUsage   = 2
)

type scrapeResult struct {
	Status    string            `json:"status"`
	Command   string            `json:"command"`
	OutputDir string            `json:"output_dir,omitempty"`
	Succeeded []string          `json:"succeeded"`
	Failed    []providerFailure `json:"failed"`
	Partial   bool              `json:"partial"`
	Error     string            `json:"error,omitempty"`
}

type providersResult struct {
	Providers []providerInfo `json:"providers"`
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	log.SetOutput(stderr)

	global := flag.NewFlagSet("ec2instances", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	jsonOutput := global.Bool("json", false, "write one JSON object to stdout")
	global.Usage = func() { writeUsage(stderr) }

	if err := global.Parse(args); err != nil {
		if err == flag.ErrHelp {
			writeUsage(stdout)
			return exitSuccess
		}
		return writeCLIError(stdout, stderr, *jsonOutput, err)
	}
	if global.NArg() == 0 {
		writeUsage(stderr)
		return exitUsage
	}

	command := global.Arg(0)
	commandArgs := global.Args()[1:]
	switch command {
	case "help", "-h", "--help":
		writeUsage(stdout)
		return exitSuccess
	case "providers":
		if len(commandArgs) != 0 {
			return writeCLIError(stdout, stderr, *jsonOutput, fmt.Errorf("providers does not accept arguments"))
		}
		result := providersResult{Providers: listProviderInfo()}
		return writeResult(stdout, stderr, *jsonOutput, result, func(w io.Writer) {
			for _, provider := range providers {
				fmt.Fprintln(w, provider.Name)
			}
		})
	case "version":
		if len(commandArgs) != 0 {
			return writeCLIError(stdout, stderr, *jsonOutput, fmt.Errorf("version does not accept arguments"))
		}
		result := versionInfo{Version: version, Commit: commit, BuildDate: buildDate}
		return writeResult(stdout, stderr, *jsonOutput, result, func(w io.Writer) {
			fmt.Fprintf(w, "ec2instances %s (%s, %s)\n", version, commit, buildDate)
		})
	case "scrape":
		return runScrape(commandArgs, stdout, stderr, *jsonOutput)
	default:
		return writeCLIError(stdout, stderr, *jsonOutput, fmt.Errorf("unknown command %q", command))
	}
}

func runScrape(args []string, stdout, stderr io.Writer, jsonOutput bool) int {
	flags := flag.NewFlagSet("scrape", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	providerNames := flags.String("providers", "", "comma-separated providers to scrape")
	outputDir := flags.String("output-dir", "", "directory for generated data")
	requestTimeout := flags.Duration("request-timeout", 0, "timeout for each HTTP request attempt")
	requestAttempts := flags.Int("request-attempts", 0, "maximum attempts for retryable HTTP requests")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			writeUsage(stdout)
			return exitSuccess
		}
		return writeScrapeError(stdout, stderr, jsonOutput, exitUsage, "", err)
	}
	if flags.NArg() != 0 {
		return writeScrapeError(stdout, stderr, jsonOutput, exitUsage, "", fmt.Errorf("unexpected arguments: %v", flags.Args()))
	}

	timeoutSet, attemptsSet := false, false
	flags.Visit(func(current *flag.Flag) {
		switch current.Name {
		case "request-timeout":
			timeoutSet = true
		case "request-attempts":
			attemptsSet = true
		}
	})
	httpConfig, err := resolveHTTPConfig(*requestTimeout, timeoutSet, *requestAttempts, attemptsSet)
	if err != nil {
		return writeScrapeError(stdout, stderr, jsonOutput, exitUsage, "", err)
	}
	if err := utils.ConfigureHTTP(httpConfig); err != nil {
		return writeScrapeError(stdout, stderr, jsonOutput, exitUsage, "", err)
	}

	if *providerNames == "" {
		*providerNames = os.Getenv("EC2INSTANCES_PROVIDERS")
	}
	if *providerNames == "" {
		*providerNames = os.Getenv("ALLOWED_SERVICES")
	}
	selected, err := selectProviders(*providerNames)
	if err != nil {
		return writeScrapeError(stdout, stderr, jsonOutput, exitUsage, "", err)
	}
	if err := validateCredentials(selected); err != nil {
		return writeScrapeError(stdout, stderr, jsonOutput, exitUsage, "", err)
	}

	if *outputDir == "" {
		*outputDir = os.Getenv("EC2INSTANCES_OUTPUT_DIR")
	}
	if *outputDir == "" {
		*outputDir = "./output"
	}
	absoluteOutputDir, err := utils.SetOutputDir(*outputDir)
	if err != nil {
		return writeScrapeError(stdout, stderr, jsonOutput, exitRuntime, "", err)
	}

	succeeded, failed := runProviders(selected)
	result := scrapeResult{
		Status:    "ok",
		Command:   "scrape",
		OutputDir: absoluteOutputDir,
		Succeeded: succeeded,
		Failed:    failed,
		Partial:   len(failed) > 0,
	}
	exitCode := exitSuccess
	if len(failed) > 0 {
		result.Status = "error"
		result.Error = "one or more providers failed"
		exitCode = exitRuntime
	}

	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "write result: %v\n", err)
			return exitRuntime
		}
	} else {
		fmt.Fprintf(stdout, "Output: %s\n", result.OutputDir)
		for _, name := range result.Succeeded {
			fmt.Fprintf(stdout, "Succeeded: %s\n", name)
		}
		for _, failure := range result.Failed {
			fmt.Fprintf(stdout, "Failed: %s: %s\n", failure.Name, failure.Error)
		}
	}
	return exitCode
}

func resolveHTTPConfig(timeout time.Duration, timeoutSet bool, attempts int, attemptsSet bool) (utils.HTTPConfig, error) {
	config := utils.DefaultHTTPConfig()

	if timeoutSet {
		config.RequestTimeout = timeout
	} else if value := os.Getenv("EC2INSTANCES_REQUEST_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config, fmt.Errorf("invalid EC2INSTANCES_REQUEST_TIMEOUT: %w", err)
		}
		config.RequestTimeout = parsed
	}

	if attemptsSet {
		config.MaxAttempts = attempts
	} else if value := os.Getenv("EC2INSTANCES_REQUEST_ATTEMPTS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return config, fmt.Errorf("invalid EC2INSTANCES_REQUEST_ATTEMPTS: %w", err)
		}
		config.MaxAttempts = parsed
	}

	if config.RequestTimeout <= 0 {
		return config, fmt.Errorf("request timeout must be greater than zero")
	}
	if config.MaxAttempts < 1 {
		return config, fmt.Errorf("request attempts must be at least 1")
	}
	return config, nil
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  ec2instances [--json] providers")
	fmt.Fprintln(w, "  ec2instances [--json] version")
	fmt.Fprintln(w, "  ec2instances [--json] scrape --providers <names> [--output-dir <path>] [--request-timeout <duration>] [--request-attempts <count>]")
}

func writeCLIError(stdout, stderr io.Writer, jsonOutput bool, err error) int {
	return writeScrapeError(stdout, stderr, jsonOutput, exitUsage, "", err)
}

func writeScrapeError(stdout, stderr io.Writer, jsonOutput bool, exitCode int, outputDir string, err error) int {
	if jsonOutput {
		result := scrapeResult{
			Status:    "error",
			Command:   "scrape",
			OutputDir: outputDir,
			Succeeded: []string{},
			Failed:    []providerFailure{},
			Partial:   false,
			Error:     err.Error(),
		}
		if encodeErr := json.NewEncoder(stdout).Encode(result); encodeErr != nil {
			fmt.Fprintf(stderr, "write result: %v\n", encodeErr)
			return exitRuntime
		}
	} else {
		fmt.Fprintln(stderr, err)
	}
	return exitCode
}

func writeResult(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	result any,
	writeText func(io.Writer),
) int {
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "write result: %v\n", err)
			return exitRuntime
		}
	} else {
		writeText(stdout)
	}
	return exitSuccess
}
