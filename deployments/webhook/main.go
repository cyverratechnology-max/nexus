package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type server struct {
	secret  []byte
	appDir  string
	update  string
	running bool
	mutex   sync.Mutex
	log     *slog.Logger
}

func main() {
	s := &server{secret: []byte(os.Getenv("GITHUB_WEBHOOK_SECRET")), appDir: env("APP_DIR", "/opt/cyverra-nexus"), update: env("UPDATE_SCRIPT", "/opt/cyverra-nexus/update.sh"), log: slog.New(slog.NewJSONHandler(os.Stdout, nil))}
	if len(s.secret) < 16 {
		s.log.Error("GITHUB_WEBHOOK_SECRET must be at least 16 characters")
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /hooks/github", s.github)
	s.log.Info("webhook listening", "addr", env("WEBHOOK_ADDR", "127.0.0.1:9090"))
	if err := http.ListenAndServe(env("WEBHOOK_ADDR", "127.0.0.1:9090"), mux); err != nil {
		s.log.Error("webhook stopped", "error", err)
		os.Exit(1)
	}
}
func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}
func (s *server) github(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-GitHub-Event") != "push" {
		http.Error(w, "ignored event", http.StatusNoContent)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "invalid body", 400)
		return
	}
	if !s.validSignature(r.Header.Get("X-Hub-Signature-256"), body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	if !strings.HasSuffix(r.Header.Get("X-GitHub-Ref"), "/main") {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	s.mutex.Lock()
	if s.running {
		s.mutex.Unlock()
		http.Error(w, "deployment already running", http.StatusConflict)
		return
	}
	s.running = true
	s.mutex.Unlock()
	w.WriteHeader(http.StatusAccepted)
	go s.deploy()
}
func (s *server) validSignature(header string, body []byte) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	supplied, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	return len(supplied) == len(expected) && subtle.ConstantTimeCompare(supplied, expected) == 1
}
func (s *server) deploy() {
	defer func() { s.mutex.Lock(); s.running = false; s.mutex.Unlock() }()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", s.update)
	command.Dir = s.appDir
	output, err := command.CombinedOutput()
	if err != nil {
		s.log.Error("deployment failed", "error", err, "output", string(output))
		return
	}
	s.log.Info("deployment completed", "output", string(output))
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
