package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"

	"nightdrive/internal/api"
	"nightdrive/internal/config"
	"nightdrive/internal/db"
	"nightdrive/internal/scanner"
	"nightdrive/internal/web"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "nightdrive.toml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("[warn] config load: %v; using defaults", err)
		cfg = config.Default()
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Database), 0o755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}

	database, err := db.Open(cfg.Database)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		log.Fatalf("migrate db: %v", err)
	}

	users, _ := database.Users()
	if len(users) == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if _, err := database.InsertUser("admin", string(hash), "admin"); err != nil {
			log.Printf("[warn] create default admin: %v", err)
		} else {
			log.Println("[nightdrive] created default admin user (admin / admin)")
		}
	}

	scan := scanner.New(cfg, database)
	if len(cfg.MusicDir) > 0 {
		for _, d := range cfg.MusicDir {
			if d == "" {
				continue
			}
			if err := scan.AddRoot(d); err != nil {
				log.Printf("[warn] add root %s: %v", d, err)
			}
		}
		go scan.Run()
	}

	srv := api.NewServer(cfg, database, scan)
	mux := http.NewServeMux()
	srv.Register(mux)
	web.Register(mux)

	addr := cfg.Listen
	log.Printf("[nightdrive] listening on http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
