package main

import (
	"fmt"
	"github.com/leafrun/leafrun-infra/internal/hydraprovision"
	"os"
)

func main() {
	if hydraprovision.Provision("http://hydra:4445", nil) != nil {
		fmt.Fprintln(os.Stderr, "Fixed Hydra client provisioning/verification failed; existing configuration preserved")
		os.Exit(1)
	}
	fmt.Println("Fixed Hydra public client verified: exact localhost callback, code+refresh only, public subject, opaque tokens, explicit scopes and audience")
}
