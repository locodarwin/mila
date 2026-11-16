package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/backup"
	"github.com/locodarwin/mila/server/internal/config"
	"github.com/locodarwin/mila/server/internal/datagen"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
)

const generateUsage = `usage: mila dev generate --email <address> [--tasks N] [--projects N]
       [--board-cards N] [--members N] [--seed N] [--workspace <name>] [settings flags]`

// devGenerate fills a new workspace with made-up work for the performance
// budgets. It may run while the server does; open devices see the new
// workspace on their next look at the workspace list.
func devGenerate(args []string) error {
	var spec datagen.Spec
	ints := map[string]*int{"tasks": &spec.Tasks, "projects": &spec.Projects, "board-cards": &spec.BoardCards, "members": &spec.Members}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		known := name == "email" || name == "seed" || name == "workspace" || ints[name] != nil
		if !strings.HasPrefix(a, "-") || !known {
			rest = append(rest, a)
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return errors.New(generateUsage)
			}
			value = args[i+1]
			i++
		}
		switch name {
		case "email":
			spec.Email = value
		case "workspace":
			spec.Workspace = value
		case "seed":
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return fmt.Errorf("--seed: %q is not a whole number", value)
			}
			spec.Seed = n
		default:
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return fmt.Errorf("--%s: %q is not a whole number", name, value)
			}
			if name == "members" && n == 0 {
				n = -1 // asked for none, rather than the default
			}
			*ints[name] = n
		}
	}
	if spec.Email == "" {
		return errors.New(generateUsage)
	}
	cfg, err := config.LoadFromOS("mila dev generate", rest)
	if err != nil {
		return err
	}
	ctx, stop := interrupted()
	defer stop()
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	log := newLogger(cfg)
	if _, err := backup.Prepare(ctx, d, cfg.BackupDir, log); err != nil {
		return err
	}
	st := store.New(d)
	start := time.Now()
	res, err := datagen.Generate(ctx, st, syncer.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), nil), spec)
	if err != nil {
		return err
	}
	fmt.Printf("workspace %s\nboard     %s/projects/%s?view=board\nowner     %s\n",
		res.WorkspaceID, strings.TrimSuffix(cfg.BaseURL, "/"), res.BoardProjectID, res.OwnerID)
	fmt.Fprintf(os.Stderr, "Made %d tasks and %d other rows in %s. Sign in with: mila user login-link %s\n",
		res.Rows["task"], res.Ops-res.Rows["task"], time.Since(start).Round(time.Millisecond), strings.ToLower(spec.Email))
	return nil
}
