package main

import (
	"github.com/leafrun/leafrun-infra/internal/fixture"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	if os.Getenv("LEAFRUN_DEV_FIXTURE") != "true" {
		log.Fatal("explicit LEAFRUN_DEV_FIXTURE=true required")
	}
	owner, admin := os.Getenv("LEAFRUN_OWNER_TOKEN"), os.Getenv("LEAFRUN_FIXTURE_ADMIN_TOKEN")
	if len(owner) < 32 || len(admin) < 32 || owner == admin {
		log.Fatal("independent owner and admin credentials required")
	}
	s := &http.Server{Addr: ":8082", Handler: fixture.New(owner, admin), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second}
	log.Fatal(s.ListenAndServe())
}
