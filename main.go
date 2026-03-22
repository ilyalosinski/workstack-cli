package main

import (
	"fmt"
	"os"

	"github.com/ilyalosinski/workstack-cli/config"
	"github.com/ilyalosinski/workstack-cli/db"
	"github.com/ilyalosinski/workstack-cli/session"
	"github.com/ilyalosinski/workstack-cli/tui"
)

func main() {
	database, err := db.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	cfg := config.Load()
	mgr := session.NewManager(database, cfg.BaseDir)

	tui.Run(mgr)
}
