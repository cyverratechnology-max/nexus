package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const authContextKey contextKey = "auth"

type auth struct{ UserID, OrganizationID, Role string }
type server struct {
	db     *pgxpool.Pool
	secret []byte
	log    *slog.Logger
}
type loginRequest struct{ Email, Password string }
type enrollRequest struct{ Token, Hostname, DeviceUUID, SerialNumber, Manufacturer, Model, Platform, OSName, OSVersion, Architecture, AgentVersion string }
type heartbeatRequest struct {
	Hostname, Platform, OSName, OSVersion, AgentVersion string
	IPAddress                                           string
}
type inventoryRequest struct {
	Hostname     string         `json:"hostname"`
	Platform     string         `json:"platform"`
	OSName       string         `json:"os_name"`
	OSVersion    string         `json:"os_version"`
	Architecture string         `json:"architecture"`
	CPUCores     int            `json:"cpu_cores"`
	MemoryTotal  uint64         `json:"memory_total_bytes"`
	MemoryUsed   uint64         `json:"memory_used_bytes"`
	Network      []networkEntry `json:"network"`
}
type networkEntry struct {
	Name string   `json:"name"`
	MAC  string   `json:"mac"`
	IPs  []string `json:"ips"`
}
type metricsRequest struct {
	CPUPercent       *float64 `json:"cpu_percent"`
	MemoryUsedBytes  *uint64  `json:"memory_used_bytes"`
	MemoryTotalBytes *uint64  `json:"memory_total_bytes"`
}
type commandRequest struct {
	Command string `json:"command"`
}
type commandResult struct {
	ID       string `json:"id"`
	Command  string `json:"command,omitempty"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()
	dsn := env("DATABASE_URL", "postgres://cyverra:cyverra@localhost:5432/cyverra?sslmode=disable")
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		logger.Error("database pool", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err = db.Ping(ctx); err != nil {
		logger.Error("database ping", "error", err)
		os.Exit(1)
	}
	if err = migrate(ctx, db); err != nil {
		logger.Error("migration", "error", err)
		os.Exit(1)
	}
	if err = bootstrapAdmin(ctx, db); err != nil {
		logger.Error("bootstrap admin", "error", err)
		os.Exit(1)
	}
	s := &server{db: db, secret: []byte(env("JWT_SECRET", "development-secret-change-me")), log: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("GET /api/v1/devices", s.requireUser(s.devices))
	mux.HandleFunc("POST /api/v1/enrollment-tokens", s.requireUser(s.createEnrollmentToken))
	mux.HandleFunc("POST /api/v1/agent/enroll", s.enroll)
	mux.HandleFunc("POST /api/v1/agent/heartbeat", s.agentHeartbeat)
	mux.HandleFunc("POST /api/v1/agent/inventory", s.agentInventory)
	mux.HandleFunc("GET /api/v1/devices/{id}/inventory", s.requireUser(s.deviceInventory))
	mux.HandleFunc("GET /api/v1/monitoring/summary", s.requireUser(s.monitoringSummary))
	mux.HandleFunc("GET /api/v1/devices/{id}/metrics", s.requireUser(s.deviceMetrics))
	mux.HandleFunc("POST /api/v1/agent/metrics", s.agentMetrics)
	mux.HandleFunc("POST /api/v1/devices/{id}/commands", s.requireUser(s.createCommand))
	mux.HandleFunc("GET /api/v1/agent/commands", s.agentCommands)
	mux.HandleFunc("POST /api/v1/agent/commands/{id}/result", s.agentCommandResult)
	mux.HandleFunc("GET /api/v1/devices/{id}/commands", s.requireUser(s.listCommands))
	handler := s.cors(s.requestID(mux))
	addr := env("API_ADDR", ":8080")
	logger.Info("api listening", "addr", addr)
	if err = http.ListenAndServe(addr, handler); err != nil {
		logger.Error("server", "error", err)
	}
}

func bootstrapAdmin(ctx context.Context, db *pgxpool.Pool) error {
	email := env("ADMIN_EMAIL", "admin@cyverra.local")
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		return fmt.Errorf("ADMIN_PASSWORD must be set")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO organizations (name, slug) VALUES ('Default Organization', 'default') ON CONFLICT (slug) DO NOTHING`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO users (organization_id, email, password_hash, role) SELECT id, $1, $2, 'org_admin' FROM organizations WHERE slug='default' ON CONFLICT (organization_id, email) DO NOTHING`, email, string(hash))
	return err
}

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	directory := env("MIGRATIONS_DIR", "backend/migrations")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		b, err := os.ReadFile(directory + string(os.PathSeparator) + entry.Name())
		if err != nil {
			return err
		}
		if _, err = db.Exec(ctx, string(b)); err != nil {
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}
	}
	return nil
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (s *server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", randomToken(12))
		next.ServeHTTP(w, r)
	})
}
func (s *server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", env("CORS_ORIGIN", "http://localhost:5173"))
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decode(r, &req) || req.Email == "" || req.Password == "" {
		writeError(w, 400, "invalid credentials")
		return
	}
	var a auth
	var hash string
	err := s.db.QueryRow(r.Context(), "SELECT id, organization_id, role, password_hash FROM users WHERE lower(email)=lower($1)", req.Email).Scan(&a.UserID, &a.OrganizationID, &a.Role, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeError(w, 401, "invalid credentials")
		return
	}
	writeJSON(w, 200, map[string]string{"token": s.sign(a), "organization_id": a.OrganizationID})
}
func (s *server) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.parseBearer(r.Header.Get("Authorization"))
		if !ok || a.UserID == "" {
			writeError(w, 401, "authentication required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), authContextKey, a)))
	}
}
func (s *server) devices(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	rows, err := s.db.Query(r.Context(), "SELECT id, hostname, platform, os_name, os_version, status, last_seen, agent_version FROM devices WHERE organization_id=$1 AND status <> 'RETIRED' ORDER BY hostname", a.OrganizationID)
	if err != nil {
		writeError(w, 500, "unable to list devices")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, hostname, platform, osName, osVersion, status, agentVersion string
		var lastSeen sql.NullTime
		if err := rows.Scan(&id, &hostname, &platform, &osName, &osVersion, &status, &lastSeen, &agentVersion); err != nil {
			writeError(w, 500, "unable to read devices")
			return
		}
		item := map[string]any{"id": id, "hostname": hostname, "platform": platform, "os_name": osName, "os_version": osVersion, "status": status, "agent_version": agentVersion}
		if lastSeen.Valid {
			item["last_seen"] = lastSeen.Time
		}
		result = append(result, item)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}
func (s *server) createEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	if a.Role != "platform_admin" && a.Role != "org_admin" && a.Role != "technician" {
		writeError(w, 403, "insufficient permission")
		return
	}
	plain := randomToken(32)
	_, err := s.db.Exec(r.Context(), "INSERT INTO enrollment_tokens (organization_id, token_hash, expires_at) VALUES ($1,$2,$3)", a.OrganizationID, hash(plain), time.Now().Add(24*time.Hour))
	if err != nil {
		writeError(w, 500, "unable to create enrollment token")
		return
	}
	writeJSON(w, 201, map[string]any{"token": plain, "expires_at": time.Now().Add(24 * time.Hour)})
}
func (s *server) enroll(w http.ResponseWriter, r *http.Request) {
	var req enrollRequest
	if !decode(r, &req) || req.Token == "" || req.DeviceUUID == "" || req.Hostname == "" {
		writeError(w, 400, "token, device_uuid, and hostname are required")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "enrollment unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var org string
	err = tx.QueryRow(r.Context(), "UPDATE enrollment_tokens SET used_at=now() WHERE token_hash=$1 AND used_at IS NULL AND expires_at>now() RETURNING organization_id", hash(req.Token)).Scan(&org)
	if err != nil {
		writeError(w, 401, "invalid or expired enrollment token")
		return
	}
	credential := randomToken(48)
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO devices (organization_id, hostname, device_uuid, serial_number, manufacturer, model, platform, os_name, os_version, architecture, agent_version, status, last_seen, credential_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'ONLINE',now(),$12) ON CONFLICT (organization_id, device_uuid) DO UPDATE SET hostname=EXCLUDED.hostname, status='ONLINE', last_seen=now(), updated_at=now(), credential_hash=EXCLUDED.credential_hash RETURNING id`, org, req.Hostname, req.DeviceUUID, req.SerialNumber, req.Manufacturer, req.Model, req.Platform, req.OSName, req.OSVersion, req.Architecture, req.AgentVersion, hash(credential)).Scan(&id)
	if err != nil {
		writeError(w, 500, "unable to register device")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "unable to complete enrollment")
		return
	}
	writeJSON(w, 201, map[string]string{"device_id": id, "credential": credential})
}
func (s *server) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req heartbeatRequest
	if !decode(r, &req) {
		writeError(w, 400, "invalid heartbeat")
		return
	}
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var id string
	err := s.db.QueryRow(r.Context(), "UPDATE devices SET status='ONLINE', last_seen=now(), hostname=COALESCE(NULLIF($2,''),hostname), agent_version=COALESCE(NULLIF($3,''),agent_version), updated_at=now() WHERE credential_hash=$1 RETURNING id", hash(credential), req.Hostname, req.AgentVersion).Scan(&id)
	if err != nil {
		writeError(w, 401, "invalid device credential")
		return
	}
	writeJSON(w, 200, map[string]any{"device_id": id, "status": "ONLINE", "received_at": time.Now()})
}
func (s *server) agentInventory(w http.ResponseWriter, r *http.Request) {
	var req inventoryRequest
	if !decode(r, &req) || req.Hostname == "" || req.Platform == "" {
		writeError(w, 400, "hostname and platform are required")
		return
	}
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	payload, err := json.Marshal(req)
	if err != nil {
		writeError(w, 400, "invalid inventory")
		return
	}
	var id string
	err = s.db.QueryRow(r.Context(), "SELECT id FROM devices WHERE credential_hash=$1", hash(credential)).Scan(&id)
	if err != nil {
		writeError(w, 401, "invalid device credential")
		return
	}
	_, err = s.db.Exec(r.Context(), `INSERT INTO device_inventory (device_id, payload, collected_at, updated_at) VALUES ($1,$2,now(),now()) ON CONFLICT (device_id) DO UPDATE SET payload=EXCLUDED.payload, collected_at=EXCLUDED.collected_at, updated_at=now()`, id, payload)
	if err != nil {
		writeError(w, 500, "unable to store inventory")
		return
	}
	writeJSON(w, 202, map[string]string{"device_id": id, "status": "accepted"})
}
func (s *server) deviceInventory(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	var id, payload string
	err := s.db.QueryRow(r.Context(), "SELECT di.device_id, di.payload::text FROM device_inventory di JOIN devices d ON d.id=di.device_id WHERE di.device_id=$1 AND d.organization_id=$2", r.PathValue("id"), a.OrganizationID).Scan(&id, &payload)
	if err != nil {
		writeError(w, 404, "inventory not found")
		return
	}
	var decoded any
	if json.Unmarshal([]byte(payload), &decoded) != nil {
		writeError(w, 500, "invalid stored inventory")
		return
	}
	writeJSON(w, 200, map[string]any{"device_id": id, "data": decoded})
}
func (s *server) agentMetrics(w http.ResponseWriter, r *http.Request) {
	var req metricsRequest
	if !decode(r, &req) {
		writeError(w, 400, "invalid metrics")
		return
	}
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var id string
	if err := s.db.QueryRow(r.Context(), "SELECT id FROM devices WHERE credential_hash=$1", hash(credential)).Scan(&id); err != nil {
		writeError(w, 401, "invalid device credential")
		return
	}
	_, err := s.db.Exec(r.Context(), "INSERT INTO device_metrics (device_id, cpu_percent, memory_used_bytes, memory_total_bytes) VALUES ($1,$2,$3,$4)", id, req.CPUPercent, req.MemoryUsedBytes, req.MemoryTotalBytes)
	if err != nil {
		writeError(w, 500, "unable to store metrics")
		return
	}
	writeJSON(w, 202, map[string]string{"device_id": id, "status": "accepted"})
}
func (s *server) monitoringSummary(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	rows, err := s.db.Query(r.Context(), `SELECT d.id, d.hostname, d.status, m.cpu_percent, m.memory_used_bytes, m.memory_total_bytes, m.collected_at FROM devices d LEFT JOIN LATERAL (SELECT cpu_percent, memory_used_bytes, memory_total_bytes, collected_at FROM device_metrics WHERE device_id=d.id ORDER BY collected_at DESC LIMIT 1) m ON true WHERE d.organization_id=$1 AND d.status <> 'RETIRED' ORDER BY d.hostname`, a.OrganizationID)
	if err != nil {
		writeError(w, 500, "unable to load monitoring data")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, hostname, status string
		var cpu, memoryUsed, memoryTotal sql.NullFloat64
		var collected sql.NullTime
		if err := rows.Scan(&id, &hostname, &status, &cpu, &memoryUsed, &memoryTotal, &collected); err != nil {
			writeError(w, 500, "unable to read monitoring data")
			return
		}
		item := map[string]any{"device_id": id, "hostname": hostname, "status": status}
		if cpu.Valid {
			item["cpu_percent"] = cpu.Float64
		}
		if memoryUsed.Valid {
			item["memory_used_bytes"] = memoryUsed.Float64
		}
		if memoryTotal.Valid {
			item["memory_total_bytes"] = memoryTotal.Float64
		}
		if collected.Valid {
			item["collected_at"] = collected.Time
		}
		result = append(result, item)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}
func (s *server) deviceMetrics(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	rows, err := s.db.Query(r.Context(), `SELECT m.cpu_percent, m.memory_used_bytes, m.memory_total_bytes, m.collected_at FROM device_metrics m JOIN devices d ON d.id=m.device_id WHERE m.device_id=$1 AND d.organization_id=$2 ORDER BY m.collected_at DESC LIMIT 60`, r.PathValue("id"), a.OrganizationID)
	if err != nil {
		writeError(w, 500, "unable to load device metrics")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var cpu, used, total sql.NullFloat64
		var collected time.Time
		if err := rows.Scan(&cpu, &used, &total, &collected); err != nil {
			writeError(w, 500, "unable to read device metrics")
			return
		}
		item := map[string]any{"collected_at": collected}
		if cpu.Valid {
			item["cpu_percent"] = cpu.Float64
		}
		if used.Valid {
			item["memory_used_bytes"] = used.Float64
		}
		if total.Valid {
			item["memory_total_bytes"] = total.Float64
		}
		result = append(result, item)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}
func (s *server) createCommand(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	if a.Role != "platform_admin" && a.Role != "org_admin" && a.Role != "technician" {
		writeError(w, 403, "insufficient permission")
		return
	}
	var req commandRequest
	if !decode(r, &req) || strings.TrimSpace(req.Command) == "" || len(req.Command) > 4096 {
		writeError(w, 400, "command must be between 1 and 4096 characters")
		return
	}
	var id string
	err := s.db.QueryRow(r.Context(), `INSERT INTO commands (organization_id, device_id, user_id, command) SELECT $1, id, $2, $3 FROM devices WHERE id=$4 AND organization_id=$1 RETURNING id`, a.OrganizationID, a.UserID, req.Command, r.PathValue("id")).Scan(&id)
	if err != nil {
		writeError(w, 404, "device not found")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO audit_logs (organization_id,user_id,device_id,action,resource,metadata) VALUES ($1,$2,$3,'command.created','command',jsonb_build_object('command_id',$4))`, a.OrganizationID, a.UserID, r.PathValue("id"), id)
	writeJSON(w, 202, map[string]string{"command_id": id, "status": "QUEUED"})
}
func (s *server) agentCommands(w http.ResponseWriter, r *http.Request) {
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var deviceID string
	if err := s.db.QueryRow(r.Context(), "SELECT id FROM devices WHERE credential_hash=$1", hash(credential)).Scan(&deviceID); err != nil {
		writeError(w, 401, "invalid device credential")
		return
	}
	rows, err := s.db.Query(r.Context(), `UPDATE commands SET status='RUNNING', started_at=now() WHERE id=(SELECT id FROM commands WHERE device_id=$1 AND status='QUEUED' ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id, status, exit_code, stdout, stderr`, deviceID)
	if err != nil {
		writeError(w, 500, "unable to load commands")
		return
	}
	defer rows.Close()
	if rows.Next() {
		var result commandResult
		if err := rows.Scan(&result.ID, &result.Command, &result.Status, &result.ExitCode, &result.Stdout, &result.Stderr); err != nil {
			writeError(w, 500, "unable to read command")
			return
		}
		writeJSON(w, 200, result)
		return
	}
	writeJSON(w, 204, nil)
}
func (s *server) agentCommandResult(w http.ResponseWriter, r *http.Request) {
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var deviceID string
	if err := s.db.QueryRow(r.Context(), "SELECT id FROM devices WHERE credential_hash=$1", hash(credential)).Scan(&deviceID); err != nil {
		writeError(w, 401, "invalid device credential")
		return
	}
	var req commandResult
	if !decode(r, &req) || req.Status == "" {
		writeError(w, 400, "invalid command result")
		return
	}
	result, err := s.db.Exec(r.Context(), `UPDATE commands SET status=$1, exit_code=$2, stdout=$3, stderr=$4, completed_at=now() WHERE id=$5 AND device_id=$6 AND status='RUNNING'`, req.Status, req.ExitCode, req.Stdout, req.Stderr, r.PathValue("id"), deviceID)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, 404, "command not found")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}
func (s *server) listCommands(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	rows, err := s.db.Query(r.Context(), `SELECT c.id,c.status,c.exit_code,c.stdout,c.stderr,c.created_at FROM commands c WHERE c.device_id=$1 AND c.organization_id=$2 ORDER BY c.created_at DESC LIMIT 50`, r.PathValue("id"), a.OrganizationID)
	if err != nil {
		writeError(w, 500, "unable to list commands")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, status, stdout, stderr string
		var code sql.NullInt32
		var created time.Time
		if err := rows.Scan(&id, &status, &code, &stdout, &stderr, &created); err != nil {
			writeError(w, 500, "unable to read commands")
			return
		}
		item := map[string]any{"id": id, "status": status, "stdout": stdout, "stderr": stderr, "created_at": created}
		if code.Valid {
			item["exit_code"] = code.Int32
		}
		result = append(result, item)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}

func (s *server) sign(a auth) string {
	payload, _ := json.Marshal(map[string]any{"uid": a.UserID, "org": a.OrganizationID, "role": a.Role, "exp": time.Now().Add(8 * time.Hour).Unix()})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + hashWithSecret(encoded, s.secret)
}
func (s *server) parseBearer(header string) (auth, bool) {
	raw := strings.TrimPrefix(header, "Bearer ")
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || subtle.ConstantTimeCompare([]byte(hashWithSecret(parts[0], s.secret)), []byte(parts[1])) != 1 {
		return auth{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return auth{}, false
	}
	var p struct {
		UID, Org, Role string
		Exp            int64
	}
	if json.Unmarshal(b, &p) != nil || p.Exp < time.Now().Unix() {
		return auth{}, false
	}
	return auth{p.UID, p.Org, p.Role}, true
}
func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func hashWithSecret(value string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}
func randomToken(size int) string {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func decode(r *http.Request, target any) bool {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(target) == nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": message}})
}
