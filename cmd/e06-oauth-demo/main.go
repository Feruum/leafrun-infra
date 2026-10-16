package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/leafrun/leafrun-infra/internal/oauthdemo"
)

func main() {
	listen := os.Getenv("E06_DEMO_LISTEN")
	if listen == "" {
		listen = "127.0.0.1:58200"
	}
	if listen != "127.0.0.1:58200" && listen != ":58200" {
		log.Fatal("fixed demo listener required")
	}
	tokenURL := os.Getenv("E06_DEMO_TOKEN_URL")
	if tokenURL == "" {
		tokenURL = "http://127.0.0.1:54444"
	}
	d, err := oauthdemo.New(oauthdemo.Config{PublicURL: "http://127.0.0.1:54444", TokenURL: tokenURL, CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate", "leafrun.status", "leafrun.source.read", "leafrun.resource.read", "leafrun.download", "offline_access"}})
	if err != nil {
		log.Fatal("demo configuration invalid")
	}
	s := &http.Server{Addr: listen, Handler: d, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, ErrorLog: log.New(io.Discard, "", 0)}
	if s.ListenAndServe() != http.ErrServerClosed {
		log.Fatal("demo listener failed")
	}
}
