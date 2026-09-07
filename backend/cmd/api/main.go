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
	"strconv"
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
	Command     string         `json:"command"`
	CommandType string         `json:"command_type"`
	Params      map[string]any `json:"params"`
	FileName    string         `json:"file_name"`
	FilePath    string         `json:"file_path"`
}
type commandResult struct {
	ID          string         `json:"id"`
	Command     string         `json:"command,omitempty"`
	CommandType string         `json:"command_type,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
	Status      string         `json:"status"`
	ExitCode    *int           `json:"exit_code,omitempty"`
	Stdout      string         `json:"stdout"`
	Stderr      string         `json:"stderr"`
	FileName    string         `json:"file_name,omitempty"`
	FilePath    string         `json:"file_path,omitempty"`
	FileContent []byte         `json:"-"`
	FileSize    *int64         `json:"file_size,omitempty"`
}
type remoteSessionRequest struct {
	Protocol   string `json:"protocol"`
	RDPHost    string `json:"rdp_host"`
	RDPPort    int    `json:"rdp_port"`
	RDPUser    string `json:"rdp_username"`
	RDPPass    string `json:"rdp_password"`
	VNCPort    int    `json:"vnc_port"`
	VNCPass    string `json:"vnc_password"`
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
	mux.HandleFunc("GET /api/v1/devices/{id}/rmm/tasks", s.requireUser(s.listRMMTasks))
	mux.HandleFunc("POST /api/v1/devices/{id}/rmm/execute", s.requireUser(s.executeRMM))
	mux.HandleFunc("POST /api/v1/agent/file-content/{id}", s.agentFileContent)
	mux.HandleFunc("GET /api/v1/devices/{id}/rmm/download/{taskId}", s.requireUser(s.downloadRMMFile))
	mux.HandleFunc("POST /api/v1/devices/{id}/remote-sessions", s.requireUser(s.createRemoteSession))
	mux.HandleFunc("GET /api/v1/devices/{id}/remote-sessions", s.requireUser(s.listRemoteSessions))
	mux.HandleFunc("POST /api/v1/remote-sessions/{id}/close", s.requireUser(s.closeRemoteSession))
	mux.HandleFunc("POST /api/v1/agent/remote-sessions/{id}/status", s.agentRemoteSessionStatus)
	mux.HandleFunc("GET /api/v1/audit-logs", s.requireUser(s.listAuditLogs))
	mux.HandleFunc("POST /api/v1/devices/{id}/unattended/token", s.requireUser(s.generateUnattendedToken))
	mux.HandleFunc("DELETE /api/v1/devices/{id}/unattended/token", s.requireUser(s.revokeUnattendedToken))
	mux.HandleFunc("PUT /api/v1/devices/{id}/unattended", s.requireUser(s.toggleUnattendedAccess))
	mux.HandleFunc("GET /api/v1/devices/unattended", s.requireUser(s.listUnattendedDevices))
	mux.HandleFunc("POST /api/v1/unattended/validate", s.validateUnattendedToken)
	handler := s.cors(s.requestID(mux))
	addr := env("API_ADDR", ":8443")
	certFile := env("TLS_CERT_FILE", "")
	keyFile := env("TLS_KEY_FILE", "")
	logger.Info("api listening", "addr", addr)
	if certFile != "" && keyFile != "" {
		logger.Info("api using TLS", "cert", certFile)
		if err = http.ListenAndServeTLS(addr, certFile, keyFile, handler); err != nil {
			logger.Error("server", "error", err)
		}
	} else {
		logger.Info("api using plain HTTP (reverse proxy should handle TLS)")
		if err = http.ListenAndServe(addr, handler); err != nil {
			logger.Error("server", "error", err)
		}
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
		w.Header().Set("Access-Control-Allow-Origin", env("CORS_ORIGIN", "https://app.cyverratech.my.id"))
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
	if req.CommandType == "" {
		req.CommandType = "execute_command"
	}
	validTypes := map[string]bool{
		"execute_command": true, "execute_script": true, "powershell": true, "cmd": true, "bash": true,
		"kill_process": true, "start_process": true,
		"restart_service": true, "stop_service": true, "start_service": true,
		"reboot": true, "shutdown": true, "lock_workstation": true, "logoff_user": true,
		"retrieve_file": true, "upload_file": true, "delete_file": true, "rename_file": true,
		"browse_filesystem": true,
	}
	if !validTypes[req.CommandType] {
		writeError(w, 400, "invalid command_type")
		return
	}
	paramsJSON := "{}"
	if req.Params != nil {
		b, _ := json.Marshal(req.Params)
		paramsJSON = string(b)
	}
	var id string
	err := s.db.QueryRow(r.Context(), `INSERT INTO commands (organization_id, device_id, user_id, command, command_type, params, file_name, file_path) SELECT $1, id, $2, $3, $4, $5::jsonb, $6, $7 FROM devices WHERE id=$8 AND organization_id=$1 RETURNING id`,
		a.OrganizationID, a.UserID, req.Command, req.CommandType, paramsJSON, req.FileName, req.FilePath, r.PathValue("id")).Scan(&id)
	if err != nil {
		writeError(w, 404, "device not found")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO audit_logs (organization_id,user_id,device_id,action,resource,metadata) VALUES ($1,$2,$3,'rmm.task.created','rmm_task',jsonb_build_object('task_id',$4,'command_type',$5))`, a.OrganizationID, a.UserID, r.PathValue("id"), id, req.CommandType)
	writeJSON(w, 202, map[string]string{"command_id": id, "status": "QUEUED"})
}
func (s *server) agentCommands(w http.ResponseWriter, r *http.Request) {
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var deviceID string
	if err := s.db.QueryRow(r.Context(), "SELECT id FROM devices WHERE credential_hash=$1", hash(credential)).Scan(&deviceID); err != nil {
		writeError(w, 401, "invalid device credential")
		return
	}
	rows, err := s.db.Query(r.Context(), `UPDATE commands SET status='RUNNING', started_at=now() WHERE id=(SELECT id FROM commands WHERE device_id=$1 AND status='QUEUED' ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id, command, command_type, params::text, exit_code, stdout, stderr, file_name, file_path`, deviceID)
	if err != nil {
		writeError(w, 500, "unable to load commands")
		return
	}
	defer rows.Close()
	if rows.Next() {
		var result commandResult
		var paramsText string
		if err := rows.Scan(&result.ID, &result.Command, &result.CommandType, &paramsText, &result.ExitCode, &result.Stdout, &result.Stderr, &result.FileName, &result.FilePath); err != nil {
			writeError(w, 500, "unable to read command")
			return
		}
		if paramsText != "" {
			_ = json.Unmarshal([]byte(paramsText), &result.Params)
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
	result, err := s.db.Exec(r.Context(), `UPDATE commands SET status=$1, exit_code=$2, stdout=$3, stderr=$4, completed_at=now(), file_name=COALESCE(NULLIF($5,''),file_name), file_path=COALESCE(NULLIF($6,''),file_path), file_size=$7 WHERE id=$8 AND device_id=$9 AND status='RUNNING'`,
		req.Status, req.ExitCode, req.Stdout, req.Stderr, req.FileName, req.FilePath, req.FileSize, r.PathValue("id"), deviceID)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, 404, "command not found")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO audit_logs (organization_id,user_id,device_id,action,resource,metadata) VALUES ((SELECT organization_id FROM devices WHERE id=$1),(SELECT user_id FROM commands WHERE id=$2),$1,'rmm.task.completed','rmm_task',jsonb_build_object('task_id',$2,'status',$3,'command_type',(SELECT command_type FROM commands WHERE id=$2)))`, deviceID, r.PathValue("id"), req.Status)
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}
func (s *server) listCommands(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	rows, err := s.db.Query(r.Context(), `SELECT c.id,c.status,c.command_type,c.params::text,c.exit_code,c.stdout,c.stderr,c.created_at,c.file_name,c.file_path,c.file_size FROM commands c WHERE c.device_id=$1 AND c.organization_id=$2 ORDER BY c.created_at DESC LIMIT 50`, r.PathValue("id"), a.OrganizationID)
	if err != nil {
		writeError(w, 500, "unable to list commands")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, status, commandType, stdout, stderr string
		var paramsText string
		var code sql.NullInt32
		var created time.Time
		var fileName, filePath sql.NullString
		var fileSize sql.NullInt64
		if err := rows.Scan(&id, &status, &commandType, &paramsText, &code, &stdout, &stderr, &created, &fileName, &filePath, &fileSize); err != nil {
			writeError(w, 500, "unable to read commands")
			return
		}
		item := map[string]any{"id": id, "status": status, "command_type": commandType, "stdout": stdout, "stderr": stderr, "created_at": created}
		if code.Valid {
			item["exit_code"] = code.Int32
		}
		if fileName.Valid {
			item["file_name"] = fileName.String
		}
		if filePath.Valid {
			item["file_path"] = filePath.String
		}
		if fileSize.Valid {
			item["file_size"] = fileSize.Int64
		}
		if paramsText != "" && paramsText != "{}" {
			var params map[string]any
			if json.Unmarshal([]byte(paramsText), &params) == nil {
				item["params"] = params
			}
		}
		result = append(result, item)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}
func (s *server) listRMMTasks(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	rows, err := s.db.Query(r.Context(), `SELECT c.id,c.status,c.command_type,c.command,c.params::text,c.exit_code,c.stdout,c.stderr,c.created_at,c.started_at,c.completed_at,c.file_name,c.file_path,c.file_size FROM commands c WHERE c.device_id=$1 AND c.organization_id=$2 AND c.command_type <> 'execute_command' ORDER BY c.created_at DESC LIMIT 100`, r.PathValue("id"), a.OrganizationID)
	if err != nil {
		writeError(w, 500, "unable to list RMM tasks")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, status, commandType, command, stdout, stderr string
		var paramsText string
		var code sql.NullInt32
		var created time.Time
		var started, completed sql.NullTime
		var fileName, filePath sql.NullString
		var fileSize sql.NullInt64
		if err := rows.Scan(&id, &status, &commandType, &command, &paramsText, &code, &stdout, &stderr, &created, &started, &completed, &fileName, &filePath, &fileSize); err != nil {
			writeError(w, 500, "unable to read RMM tasks")
			return
		}
		item := map[string]any{"id": id, "status": status, "command_type": commandType, "command": command, "stdout": stdout, "stderr": stderr, "created_at": created}
		if code.Valid {
			item["exit_code"] = code.Int32
		}
		if started.Valid {
			item["started_at"] = started.Time
		}
		if completed.Valid {
			item["completed_at"] = completed.Time
		}
		if fileName.Valid {
			item["file_name"] = fileName.String
		}
		if filePath.Valid {
			item["file_path"] = filePath.String
		}
		if fileSize.Valid {
			item["file_size"] = fileSize.Int64
		}
		if paramsText != "" && paramsText != "{}" {
			var params map[string]any
			if json.Unmarshal([]byte(paramsText), &params) == nil {
				item["params"] = params
			}
		}
		result = append(result, item)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}
func (s *server) executeRMM(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	if a.Role != "platform_admin" && a.Role != "org_admin" && a.Role != "technician" {
		writeError(w, 403, "insufficient permission")
		return
	}
	var req commandRequest
	if !decode(r, &req) {
		writeError(w, 400, "invalid request")
		return
	}
	if req.CommandType == "" {
		writeError(w, 400, "command_type is required")
		return
	}
	validTypes := map[string]bool{
		"powershell": true, "cmd": true, "bash": true,
		"execute_command": true, "execute_script": true,
		"kill_process": true, "start_process": true,
		"restart_service": true, "stop_service": true, "start_service": true,
		"reboot": true, "shutdown": true, "lock_workstation": true, "logoff_user": true,
		"retrieve_file": true, "upload_file": true, "delete_file": true, "rename_file": true,
		"browse_filesystem": true,
	}
	if !validTypes[req.CommandType] {
		writeError(w, 400, "invalid command_type")
		return
	}
	paramsJSON := "{}"
	if req.Params != nil {
		b, _ := json.Marshal(req.Params)
		paramsJSON = string(b)
	}
	var id string
	err := s.db.QueryRow(r.Context(), `INSERT INTO commands (organization_id, device_id, user_id, command, command_type, params, file_name, file_path) SELECT $1, id, $2, $3, $4, $5::jsonb, $6, $7 FROM devices WHERE id=$8 AND organization_id=$1 AND status <> 'RETIRED' RETURNING id`,
		a.OrganizationID, a.UserID, req.Command, req.CommandType, paramsJSON, req.FileName, req.FilePath, r.PathValue("id")).Scan(&id)
	if err != nil {
		writeError(w, 404, "device not found or offline")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO audit_logs (organization_id,user_id,device_id,action,resource,metadata) VALUES ($1,$2,$3,'rmm.task.created','rmm_task',jsonb_build_object('task_id',$4,'command_type',$5,'params',$6))`, a.OrganizationID, a.UserID, r.PathValue("id"), id, req.CommandType, paramsJSON)
	writeJSON(w, 202, map[string]string{"task_id": id, "status": "QUEUED"})
}
func (s *server) agentFileContent(w http.ResponseWriter, r *http.Request) {
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var deviceID string
	if err := s.db.QueryRow(r.Context(), "SELECT id FROM devices WHERE credential_hash=$1", hash(credential)).Scan(&deviceID); err != nil {
		writeError(w, 401, "invalid device credential")
		return
	}
	taskID := r.PathValue("id")
	content, err := io.ReadAll(io.LimitReader(r.Body, 100<<20))
	if err != nil {
		writeError(w, 400, "unable to read file content")
		return
	}
	result, err := s.db.Exec(r.Context(), `UPDATE commands SET file_content=$1, file_size=$2, completed_at=now(), status='SUCCESS' WHERE id=$3 AND device_id=$4 AND status='RUNNING'`, content, len(content), taskID, deviceID)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, 404, "command not found")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}
func (s *server) downloadRMMFile(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	var fileName, filePath string
	var fileContent []byte
	var fileSize sql.NullInt64
	err := s.db.QueryRow(r.Context(), `SELECT file_name, file_path, file_content, file_size FROM commands WHERE id=$1 AND device_id=$2 AND organization_id=$3 AND status='SUCCESS'`,
		r.PathValue("taskId"), r.PathValue("id"), a.OrganizationID).Scan(&fileName, &filePath, &fileContent, &fileSize)
	if err != nil {
		writeError(w, 404, "file not found")
		return
	}
	if len(fileContent) == 0 {
		writeError(w, 404, "file content not available")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	w.Header().Set("Content-Type", "application/octet-stream")
	if fileSize.Valid {
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize.Int64, 10))
	}
	_, _ = w.Write(fileContent)
}
func (s *server) createRemoteSession(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	if a.Role != "platform_admin" && a.Role != "org_admin" && a.Role != "technician" {
		writeError(w, 403, "insufficient permission")
		return
	}
	var req remoteSessionRequest
	if !decode(r, &req) {
		req.Protocol = "WEBRTC"
	}
	validProtocols := map[string]bool{"WEBRTC": true, "VNC": true, "RDP": true}
	if !validProtocols[req.Protocol] {
		writeError(w, 400, "protocol must be WEBRTC, VNC, or RDP")
		return
	}
	if req.Protocol == "RDP" && req.RDPPort == 0 {
		req.RDPPort = 3389
	}
	if req.Protocol == "VNC" && req.VNCPort == 0 {
		req.VNCPort = 5900
	}
	var id, deviceIP string
	err := s.db.QueryRow(r.Context(), `INSERT INTO remote_sessions (organization_id,device_id,user_id,protocol,stun_url,turn_url,rdp_port,rdp_username,rdp_password,vnc_port,vnc_password) SELECT $1,id,$2,$3,$4,$5,$6,$7,$8,$9,$10 FROM devices WHERE id=$11 AND organization_id=$1 AND status <> 'RETIRED' RETURNING id`,
		a.OrganizationID, a.UserID, req.Protocol, os.Getenv("STUN_URL"), os.Getenv("TURN_URL"),
		req.RDPPort, req.RDPUser, req.RDPPass, req.VNCPort, req.VNCPass, r.PathValue("id")).Scan(&id)
	if err != nil {
		writeError(w, 404, "device not found")
		return
	}
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(ip_address::text, '') FROM devices WHERE id=$1`, r.PathValue("id")).Scan(&deviceIP)
	_, _ = s.db.Exec(r.Context(), `INSERT INTO audit_logs (organization_id,user_id,device_id,action,resource,metadata) VALUES ($1,$2,$3,'remote.session.requested','remote_session',jsonb_build_object('session_id',$4,'protocol',$5))`, a.OrganizationID, a.UserID, r.PathValue("id"), id, req.Protocol)

	resp := map[string]any{"session_id": id, "status": "REQUESTED", "protocol": req.Protocol, "expires_in_seconds": 1800}
	if req.Protocol == "RDP" {
		resp["host"] = deviceIP
		resp["port"] = req.RDPPort
		resp["username"] = req.RDPUser
	} else if req.Protocol == "VNC" {
		resp["host"] = deviceIP
		resp["port"] = req.VNCPort
	}
	writeJSON(w, 202, resp)
}
func (s *server) listRemoteSessions(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	rows, err := s.db.Query(r.Context(), `SELECT id,status,protocol,created_at,expires_at,started_at,closed_at,close_reason FROM remote_sessions WHERE device_id=$1 AND organization_id=$2 ORDER BY created_at DESC LIMIT 20`, r.PathValue("id"), a.OrganizationID)
	if err != nil {
		writeError(w, 500, "unable to list remote sessions")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, status, protocol string
		var created, expires time.Time
		var started, closed sql.NullTime
		var reason sql.NullString
		if err := rows.Scan(&id, &status, &protocol, &created, &expires, &started, &closed, &reason); err != nil {
			writeError(w, 500, "unable to read remote sessions")
			return
		}
		item := map[string]any{"id": id, "status": status, "protocol": protocol, "created_at": created, "expires_at": expires}
		if started.Valid {
			item["started_at"] = started.Time
		}
		if closed.Valid {
			item["closed_at"] = closed.Time
		}
		if reason.Valid {
			item["close_reason"] = reason.String
		}
		result = append(result, item)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}
func (s *server) closeRemoteSession(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decode(r, &req)
	result, err := s.db.Exec(r.Context(), `UPDATE remote_sessions SET status='CLOSED', closed_at=now(), close_reason=$1 WHERE id=$2 AND organization_id=$3 AND status IN ('REQUESTED','APPROVED','CONNECTING','ACTIVE')`, req.Reason, r.PathValue("id"), a.OrganizationID)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, 404, "remote session not found or already closed")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "CLOSED"})
}
func (s *server) agentRemoteSessionStatus(w http.ResponseWriter, r *http.Request) {
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var deviceID string
	if err := s.db.QueryRow(r.Context(), "SELECT id FROM devices WHERE credential_hash=$1", hash(credential)).Scan(&deviceID); err != nil {
		writeError(w, 401, "invalid device credential")
		return
	}
	var req struct {
		Status        string `json:"status"`
		ConnectionURL string `json:"connection_url"`
		RelayToken    string `json:"relay_token"`
	}
	if !decode(r, &req) {
		writeError(w, 400, "invalid request")
		return
	}
	sessionID := r.PathValue("id")
	result, err := s.db.Exec(r.Context(), `UPDATE remote_sessions SET status=$1, connection_url=COALESCE(NULLIF($2,''),connection_url), relay_token=COALESCE(NULLIF($3,''),relay_token), started_at=CASE WHEN $1='ACTIVE' THEN now() ELSE started_at END WHERE id=$4 AND device_id=$5`,
		req.Status, req.ConnectionURL, req.RelayToken, sessionID, deviceID)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, 404, "session not found")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}
func (s *server) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	rows, err := s.db.Query(r.Context(), `SELECT id, user_id, device_id, action, resource, metadata::text, created_at FROM audit_logs WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 200`, a.OrganizationID)
	if err != nil {
		writeError(w, 500, "unable to list audit logs")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id int64
		var userID, deviceID, action, resource string
		var metaText string
		var created time.Time
		if err := rows.Scan(&id, &userID, &deviceID, &action, &resource, &metaText, &created); err != nil {
			writeError(w, 500, "unable to read audit logs")
			return
		}
		item := map[string]any{"id": id, "action": action, "resource": resource, "created_at": created}
		if userID != "" { item["user_id"] = userID }
		if deviceID != "" { item["device_id"] = deviceID }
		if metaText != "" && metaText != "{}" {
			var meta map[string]any
			if json.Unmarshal([]byte(metaText), &meta) == nil { item["metadata"] = meta }
		}
		result = append(result, item)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}

func (s *server) generateUnattendedToken(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	if a.Role != "platform_admin" && a.Role != "org_admin" && a.Role != "technician" {
		writeError(w, 403, "insufficient permission")
		return
	}
	deviceID := r.PathValue("id")
	plainToken := "ua_" + randomToken(32)
	tokenHash := hash(plainToken)
	result, err := s.db.Exec(r.Context(), `UPDATE devices SET unattended_token_hash=$1, unattended_enabled=true, updated_at=now() WHERE id=$2 AND organization_id=$3`,
		tokenHash, deviceID, a.OrganizationID)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, 404, "device not found")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO audit_logs (organization_id,user_id,device_id,action,resource,metadata) VALUES ($1,$2,$3,'unattended.token.generated','device',jsonb_build_object('device_id',$4))`, a.OrganizationID, a.UserID, a.OrganizationID, deviceID)
	writeJSON(w, 201, map[string]any{"token": plainToken, "message": "Save this token securely. It will not be shown again."})
}

func (s *server) revokeUnattendedToken(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	deviceID := r.PathValue("id")
	result, err := s.db.Exec(r.Context(), `UPDATE devices SET unattended_token_hash=NULL, unattended_enabled=false, updated_at=now() WHERE id=$1 AND organization_id=$2`,
		deviceID, a.OrganizationID)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, 404, "device not found")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO audit_logs (organization_id,user_id,device_id,action,resource,metadata) VALUES ($1,$2,$3,'unattended.token.revoked','device',jsonb_build_object('device_id',$4))`, a.OrganizationID, a.UserID, a.OrganizationID, deviceID)
	writeJSON(w, 200, map[string]string{"status": "revoked"})
}

func (s *server) toggleUnattendedAccess(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	if a.Role != "platform_admin" && a.Role != "org_admin" && a.Role != "technician" {
		writeError(w, 403, "insufficient permission")
		return
	}
	deviceID := r.PathValue("id")
	var req struct {
		Enabled  bool   `json:"enabled"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
	}
	if !decode(r, &req) {
		writeError(w, 400, "invalid request")
		return
	}
	if req.Protocol == "" { req.Protocol = "VNC" }
	if req.Port == 0 {
		if req.Protocol == "RDP" { req.Port = 3389 } else { req.Port = 5900 }
	}
	result, err := s.db.Exec(r.Context(), `UPDATE devices SET unattended_enabled=$1, unattended_port=$2, unattended_protocol=$3, updated_at=now() WHERE id=$4 AND organization_id=$5`,
		req.Enabled, req.Port, req.Protocol, deviceID, a.OrganizationID)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, 404, "device not found")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO audit_logs (organization_id,user_id,device_id,action,resource,metadata) VALUES ($1,$2,$3,'unattended.access.toggled','device',jsonb_build_object('device_id',$4,'enabled',$5,'protocol',$6))`, a.OrganizationID, a.UserID, a.OrganizationID, deviceID, req.Enabled, req.Protocol)
	writeJSON(w, 200, map[string]any{"enabled": req.Enabled, "port": req.Port, "protocol": req.Protocol})
}

func (s *server) listUnattendedDevices(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(authContextKey).(auth)
	rows, err := s.db.Query(r.Context(), `SELECT d.id, d.hostname, d.platform, d.status, d.unattended_enabled, d.unattended_port, d.unattended_protocol, d.unattended_last_used FROM devices d WHERE d.organization_id=$1 AND d.unattended_enabled=true AND d.status <> 'RETIRED' ORDER BY d.hostname`, a.OrganizationID)
	if err != nil {
		writeError(w, 500, "unable to list unattended devices")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, hostname, platform, status, protocol string
		var port int
		var enabled bool
		var lastUsed sql.NullTime
		if err := rows.Scan(&id, &hostname, &platform, &status, &enabled, &port, &protocol, &lastUsed); err != nil {
			writeError(w, 500, "unable to read devices")
			return
		}
		item := map[string]any{"id": id, "hostname": hostname, "platform": platform, "status": status, "enabled": enabled, "port": port, "protocol": protocol}
		if lastUsed.Valid { item["last_used"] = lastUsed.Time }
		result = append(result, item)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}

func (s *server) validateUnattendedToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		DeviceID string `json:"device_id"`
	}
	if !decode(r, &req) || req.Token == "" {
		writeError(w, 400, "token is required")
		return
	}
	tokenHash := hash(req.Token)
	var deviceID, hostname, platform, protocol string
	var port int
	var ipAddr sql.NullString
	err := s.db.QueryRow(r.Context(), `SELECT id, hostname, platform, unattended_port, unattended_protocol, ip_address::text FROM devices WHERE unattended_token_hash=$1 AND unattended_enabled=true AND status='ONLINE'`,
		tokenHash).Scan(&deviceID, &hostname, &platform, &port, &protocol, &ipAddr)
	if err != nil {
		_, _ = s.db.Exec(r.Context(), `INSERT INTO unattended_access_logs (action, token_hash, metadata) VALUES ('failed', $1, jsonb_build_object('reason','invalid token'))`, tokenHash)
		writeError(w, 401, "invalid or expired unattended access token")
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE devices SET unattended_last_used=now() WHERE id=$1`, deviceID)
	_, _ = s.db.Exec(r.Context(), `INSERT INTO unattended_access_logs (organization_id, device_id, action, token_hash, metadata) VALUES ((SELECT organization_id FROM devices WHERE id=$1), $1, 'success', $2, jsonb_build_object('hostname',$3,'protocol',$4))`, deviceID, tokenHash, hostname, protocol)
	ip := "unknown"
	if ipAddr.Valid { ip = ipAddr.String }
	writeJSON(w, 200, map[string]any{"device_id": deviceID, "hostname": hostname, "platform": platform, "ip": ip, "port": port, "protocol": protocol})
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
