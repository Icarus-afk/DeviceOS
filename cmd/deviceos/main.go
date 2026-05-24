package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	subcommand := os.Args[1]

	switch subcommand {
	case "start":
		fs := flag.NewFlagSet("start", flag.ExitOnError)
		configPath := fs.String("config", "deviceos.yaml", "path to config file")
		fs.Parse(os.Args[2:])
		cmdStart(*configPath)

	case "init":
		cmdInit()

	case "version", "--version", "-v":
		cmdVersion()

	case "status":
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		configPath := fs.String("config", "deviceos.yaml", "path to config file")
		fs.Parse(os.Args[2:])
		cmdStatus(*configPath)

	case "help", "--help", "-h":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`DeviceOS — self-hosted IoT backend

Usage:
  deviceos <command> [options]

Commands:
  start          Start the DeviceOS server
  init           Scaffold a new DeviceOS project
  status         Show server health status
  version        Print version information
  help           Show this usage message

Run 'deviceos start --help' for server flags.
`)
}
