// Command tachtop is a fast, friendly terminal system monitor for Linux.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tachtop/tachtop/internal/config"
	"github.com/tachtop/tachtop/internal/theme"
	"github.com/tachtop/tachtop/internal/ui"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tachtop:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("tachtop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	themeFlag := fs.String("theme", "", "colour theme (default: from config, or tachtop)")
	intervalFlag := fs.Duration("interval", 0, "refresh interval, e.g. 1s or 1500ms (default: from config, or 1.5s)")
	configFlag := fs.String("config", "", "config file path (default: ~/.config/tachtop/config.json)")
	listThemes := fs.Bool("list-themes", false, "list the built-in themes and exit")
	showVersion := fs.Bool("version", false, "print the version and exit")

	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintln(out, "tachtop — a fast, friendly terminal system monitor for Linux")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Usage:")
		fmt.Fprintln(out, "  tachtop [flags]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Keys:")
		fmt.Fprintln(out, "  q quit · p pause · / filter · s sort · k kill · enter details · ? help")
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (try --help)", fs.Arg(0))
	}
	if *showVersion {
		fmt.Printf("tachtop %s\n", version)
		return nil
	}
	if *listThemes {
		for _, t := range theme.All() {
			suffix := ""
			if t.Name == "tachtop" {
				suffix = "  (default)"
			}
			fmt.Printf("  %-9s %s%s\n", t.Name, t.Description, suffix)
		}
		return nil
	}

	cfg, _, err := config.Load(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tachtop: warning: %v; using defaults\n", err)
	}
	if *themeFlag != "" {
		cfg.Theme = *themeFlag
	}
	if *intervalFlag > 0 {
		cfg.RefreshMS = int(intervalFlag.Milliseconds())
		if cfg.RefreshMS < config.MinRefreshMS {
			cfg.RefreshMS = config.MinRefreshMS
		}
		if cfg.RefreshMS > config.MaxRefreshMS {
			cfg.RefreshMS = config.MaxRefreshMS
		}
	}

	pal, ok := theme.Get(cfg.Theme)
	if !ok {
		fmt.Fprintf(os.Stderr, "tachtop: unknown theme %q (available: %s); using tachtop\n",
			cfg.Theme, strings.Join(theme.Names(), ", "))
		pal, _ = theme.Get("tachtop")
	}

	model := ui.New(ui.Options{Config: cfg, Theme: pal, Version: version})
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-signals
		program.Quit()
	}()

	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run ui: %w", err)
	}
	return nil
}
