// Command owner-router routes internal stand checks to their explicit owners.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/leafrun/leafrun-infra/internal/ownerrouter"
)

func main() {
	server, err := ownerrouter.New(ownerrouter.Config{AccessMode: os.Getenv("LEAFRUN_OWNER_AUTH_MODE") == "access", DevFixture: os.Getenv("LEAFRUN_DEV_FIXTURE") == "true", FixtureURL: os.Getenv("LEAFRUN_ACCESS_URL"), FixtureToken: os.Getenv("LEAFRUN_ACCESS_TOKEN"), GenerationURL: os.Getenv("LEAFRUN_GENERATION_URL"), GenerationToken: os.Getenv("LEAFRUN_GENERATION_OWNER_TOKEN"), StorageToken: os.Getenv("LEAFRUN_OWNER_TOKEN"), ProjectsURL: os.Getenv("LEAFRUN_PROJECTS_URL"), ProjectsToken: os.Getenv("LEAFRUN_PROJECTS_OWNER_TOKEN")}, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	address := os.Getenv("LEAFRUN_OWNER_LISTEN")
	if address == "" {
		address = ":8082"
	}
	httpServer := http.Server{Addr: address, Handler: server, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	if err = httpServer.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
