package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/starloader/backend/internal/security"
)

func main() {
	secret := strings.TrimSpace(os.Args[1])
	code, err := security.TOTPCode(secret, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(code)
}
