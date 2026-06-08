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
	"nightdrive/internal/crashreport"
	"nightdrive/internal/db"
	"nightdrive/internal/logging"
	"nightdrive/internal/recovery"
	"nightdrive/internal/scanner"
	"nightdrive/internal/web"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "nightdrive.toml", "path to config file")
	flag.Parse()

	logger, err := logging.NewFileLogger("data/logs", "nightdrive", logging.InfoLevel, 10<<20)
	if err != nil {
		log.Printf("[warn] structured logging init failed: %v; falling back to stderr", err)
		logger = logging.New(os.Stderr, logging.InfoLevel)
	}
	log.SetOutput(logger.GoStdlibAdapter().Writer())

	migrator := config.NewMigrator()
	_, startVersion, err := migrator.MigrateFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		logger.Warn("config migration failed", map[string]interface{}{"error": err.Error()})
	} else if startVersion > 0 {
		logger.Info("config migrated", map[string]interface{}{"from_version": startVersion})
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Warn("config load failed; using defaults", map[string]interface{}{"error": err.Error()})
		cfg = config.Default()
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Database), 0o755); err != nil {
		logger.Fatal("mkdir data", map[string]interface{}{"error": err.Error()})
	}

	database, err := db.Open(cfg.Database)
	if err != nil {
		logger.Fatal("open db", map[string]interface{}{"error": err.Error()})
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		logger.Fatal("migrate db", map[string]interface{}{"error": err.Error()})
	}

	users, _ := database.Users()
	if len(users) == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if _, err := database.InsertUser("admin", string(hash), "admin"); err != nil {
			logger.Warn("create default admin", map[string]interface{}{"error": err.Error()})
		} else {
			logger.Info("created default admin user (admin / admin)")
		}
	}

	scan := scanner.New(cfg, database)
	if len(cfg.MusicDir) > 0 {
		for _, d := range cfg.MusicDir {
			if d == "" {
				continue
			}
			if err := scan.AddRoot(d); err != nil {
				logger.Warn("add root", map[string]interface{}{"root": d, "error": err.Error()})
			}
		}
		go scan.Run()
	}

	recMgr := recovery.NewManager("data/crashes")
	crashReporter := crashreport.NewReporter()
	_ = crashReporter.ScanCrashDumps("data/crashes")
	recMgr.SetOnRecovered(crashReporter.ObserveRecovery)

	srv := api.NewServer(cfg, database, scan, crashReporter)
	mux := http.NewServeMux()

	srv.Register(mux)
	web.Register(mux)

	handler := recMgr.Middleware(mux)

	addr := cfg.Listen
	logger.Info("listening", map[string]interface{}{"addr": addr})
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Fatal("serve", map[string]interface{}{"error": err.Error()})
	}
}
