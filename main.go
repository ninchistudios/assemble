// Command assemble helps a small group find days when everyone is available.
// See agents.md for the design.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	// Embedded zone database, so the group timezone resolves on any base
	// image, distroless included. About 450KB; worth it for never having to
	// think about it.
	_ "time/tzdata"

	"github.com/ninchistudios/assemble/internal/config"
	"github.com/ninchistudios/assemble/internal/web"
)

func main() {
	// distroless carries no shell and no curl, so the container healthcheck
	// re-executes this binary instead of shelling out.
	healthcheck := flag.Bool("healthcheck", false, "probe the local listener and exit")
	flag.Parse()
	if *healthcheck {
		os.Exit(probe())
	}

	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	handler, err := web.New(cfg)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("assemble %s listening on %s for %s", cfg.Version, cfg.Listen, cfg.BaseURL)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// probe returns a process exit code: 0 if the local listener answers /healthz.
func probe() int {
	listen := os.Getenv("ASSEMBLE_LISTEN")
	if listen == "" {
		listen = ":8080"
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		port = "8080"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.Status)
		return 1
	}
	return 0
}
