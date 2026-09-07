package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const agentVersion = "0.2.0"

var logMessage = func(message string) { fmt.Fprintln(os.Stderr, message) }

type config struct{ Token, API, Credential string }
type enrollRequest struct{ Token, Hostname, DeviceUUID, Platform, OSName, Architecture, AgentVersion string }
type enrollResponse struct{ DeviceID, Credential string }
type inventory struct {
	Hostname     string         `json:"hostname"`
	Platform     string         `json:"platform"`
	OSName       string         `json:"os_name"`
	Architecture string         `json:"architecture"`
	CPUCores     int            `json:"cpu_cores"`
	MemoryTotal  uint64         `json:"memory_total_bytes"`
	MemoryUsed   uint64         `json:"memory_used_bytes"`
	Network      []networkEntry `json:"network"`
	Disks        []diskEntry    `json:"disks"`
	Software     []string       `json:"software"`
	Services     []serviceEntry `json:"services"`
}
type networkEntry struct {
	Name string   `json:"name"`
	MAC  string   `json:"mac"`
	IPs  []string `json:"ips"`
}
type diskEntry struct {
	Name       string `json:"name"`
	SizeBytes  uint64 `json:"size_bytes,omitempty"`
	UsedBytes  uint64 `json:"used_bytes,omitempty"`
	FreeBytes  uint64 `json:"free_bytes,omitempty"`
	FileSystem string `json:"filesystem,omitempty"`
}
type serviceEntry struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}
type metrics struct {
	CPUPercent       *float64 `json:"cpu_percent,omitempty"`
	MemoryUsedBytes  *uint64  `json:"memory_used_bytes,omitempty"`
	MemoryTotalBytes *uint64  `json:"memory_total_bytes,omitempty"`
}

func main() {
	api := flag.String("api", "https://app.cyverratech.my.id", "Cyverra API base URL")
	token := flag.String("enrollment-token", os.Getenv("CYVERRA_ENROLLMENT_TOKEN"), "single-use enrollment token")
	state := flag.String("state", defaultStatePath(), "agent state file")
	once := flag.Bool("once", false, "send one heartbeat and exit")
	flag.Parse()
	if err := os.MkdirAll(filepath.Dir(*state), 0700); err != nil {
		fatal("cannot create state directory: " + err.Error())
	}
	logPath := *state + ".log"
	logFile, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if logErr == nil {
		defer logFile.Close()
		logMessage = func(message string) {
			timestamped := time.Now().Format(time.RFC3339) + " " + message + "\n"
			_, _ = logFile.WriteString(timestamped)
			fmt.Fprint(os.Stderr, timestamped)
		}
	}
	cfg := config{API: strings.TrimRight(*api, "/")}
	if data, err := os.ReadFile(*state); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	cfg.API = strings.TrimRight(*api, "/")
	if cfg.Credential == "" {
		if *token == "" {
			fatal("enrollment token is required for first run; use --enrollment-token or CYVERRA_ENROLLMENT_TOKEN")
		}
		cfg.Credential = enroll(cfg.API, *token, *state)
	}
	if !*once && checkForUpdate(cfg) {
		return
	}
	sendHeartbeat(cfg)
	sendInventory(cfg)
	sendMetrics(cfg)
	pollCommands(cfg)
	pollRemoteSessions(cfg)
	go startRemoteDesktopWS(cfg)
	go manageMeshCentralAgent(cfg)
	if *once {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if checkForUpdate(cfg) {
			return
		}
		sendHeartbeat(cfg)
		sendInventory(cfg)
		sendMetrics(cfg)
		pollCommands(cfg)
		pollRemoteSessions(cfg)
	}
}

type updateManifest struct {
	Version            string `json:"version"`
	LinuxAMD64         string `json:"linux_amd64"`
	LinuxAMD64SHA256   string `json:"linux_amd64_sha256"`
	WindowsAMD64       string `json:"windows_amd64"`
	WindowsAMD64SHA256 string `json:"windows_amd64_sha256"`
}

func checkForUpdate(cfg config) bool {
	var manifest updateManifest
	if err := get(cfg.API+"/downloads/agent-manifest.json", "", &manifest); err != nil || !newerVersion(manifest.Version, agentVersion) {
		return false
	}
	url, expected := manifest.LinuxAMD64, manifest.LinuxAMD64SHA256
	if runtime.GOOS == "windows" {
		url, expected = manifest.WindowsAMD64, manifest.WindowsAMD64SHA256
	}
	if url == "" || expected == "" {
		return false
	}
	if strings.HasPrefix(url, "/") {
		url = cfg.API + url
	}
	data, err := download(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent update unavailable: %v\n", err)
		return false
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
		fmt.Fprintln(os.Stderr, "agent update rejected: checksum mismatch")
		return false
	}
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	staged := executable + ".update"
	if err := os.WriteFile(staged, data, 0700); err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return stageWindowsUpdate(executable, staged, cfg.API)
	}
	if err := os.Rename(staged, executable); err != nil {
		fmt.Fprintf(os.Stderr, "agent update install failed: %v\n", err)
		return false
	}
	args := append([]string{executable}, os.Args[1:]...)
	if err := exec.Command(executable, args[1:]...).Start(); err != nil {
		return false
	}
	return true
}
func newerVersion(candidate, current string) bool {
	parse := func(value string) [3]int {
		var result [3]int
		parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
		for index := 0; index < len(parts) && index < 3; index++ {
			result[index], _ = strconv.Atoi(parts[index])
		}
		return result
	}
	left, right := parse(candidate), parse(current)
	for index := 0; index < 3; index++ {
		if left[index] != right[index] {
			return left[index] > right[index]
		}
	}
	return false
}
func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	result, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	if result.StatusCode >= 300 {
		return nil, fmt.Errorf("download returned %s", result.Status)
	}
	return io.ReadAll(io.LimitReader(result.Body, 100<<20))
}
func stageWindowsUpdate(executable, staged, api string) bool {
	script := staged + ".cmd"
	content := fmt.Sprintf("@echo off\r\ntimeout /t 2 /nobreak >nul\r\nmove /Y \"%s\" \"%s\" >nul\r\nstart \"\" \"%s\" --api \"%s\" --state \"%s\"\r\ndel \"%%~f0\"\r\n", staged, executable, executable, api, defaultStatePath())
	if err := os.WriteFile(script, []byte(content), 0600); err != nil {
		return false
	}
	return exec.Command("cmd.exe", "/C", script).Start() == nil
}

type commandJob struct {
	ID          string         `json:"id"`
	Command     string         `json:"command,omitempty"`
	CommandType string         `json:"command_type,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
	FileName    string         `json:"file_name,omitempty"`
	FilePath    string         `json:"file_path,omitempty"`
}

func pollCommands(cfg config) {
	var job commandJob
	if err := get(cfg.API+"/api/v1/agent/commands", cfg.Credential, &job); err != nil || job.ID == "" {
		return
	}
	result := executeRMMJob(cfg, job)
	if result != nil {
		_ = post(cfg.API+"/api/v1/agent/commands/"+job.ID+"/result", cfg.Credential, result, &map[string]any{})
	}
}

type remoteSessionJob struct {
	ID       string `json:"id"`
	Protocol string `json:"protocol"`
	Status   string `json:"status"`
}

func pollRemoteSessions(cfg config) {
	var job remoteSessionJob
	if err := get(cfg.API+"/api/v1/agent/remote-sessions", cfg.Credential, &job); err != nil || job.ID == "" {
		return
	}
	report := map[string]string{"status": "ACTIVE"}
	if job.Protocol == "RDP" {
		port := getRDPPort()
		report["connection_url"] = fmt.Sprintf("rdp://%s:%d", getLocalIP(), port)
	} else if job.Protocol == "VNC" {
		port := getVNCPort()
		report["connection_url"] = fmt.Sprintf("vnc://%s:%d", getLocalIP(), port)
	}
	_ = post(cfg.API+"/api/v1/agent/remote-sessions/"+job.ID+"/status", cfg.Credential, report, &map[string]any{})
}

func getRDPPort() int {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "(Get-ItemProperty 'HKLM:\\System\\CurrentControlSet\\Control\\Terminal Server\\WinStations\\RDP-Tcp' -Name PortNumber).PortNumber").Output()
		if err == nil {
			port := 0
			fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &port)
			if port > 0 {
				return port
			}
		}
	}
	return 3389
}

func getVNCPort() int {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("sh", "-c", "ss -tlnp | grep -i vnc | head -1 | awk '{print $4}' | rev | cut -d: -f1 | rev").Output()
		if err == nil {
			port := 0
			fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &port)
			if port > 0 {
				return port
			}
		}
	}
	return 5900
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return "127.0.0.1"
}

func startRemoteDesktopWS(cfg config) {
	for {
		time.Sleep(5 * time.Second)
		runRemoteDesktopWS(cfg)
	}
}

func runRemoteDesktopWS(cfg config) {
	wsURL := strings.Replace(cfg.API, "https://", "wss://", -1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", -1)
	wsURL += "/api/v1/agent/remote-desktop"

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	ws, _, err := dialer.Dial(wsURL, http.Header{"Authorization": []string{"Bearer " + cfg.Credential}})
	if err != nil {
		return
	}
	defer ws.Close()
	logMessage("remote desktop websocket connected")

	var mu sync.Mutex
	capturing := false
	stopCapture := make(chan struct{})

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return
		}
		var msg struct {
			Type      string          `json:"type"`
			SessionID string          `json:"session_id"`
			Protocol  string          `json:"protocol"`
			Data      json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "start_session":
			mu.Lock()
			if !capturing {
				capturing = true
				stopCapture = make(chan struct{})
				go captureAndStream(ws, msg.SessionID, stopCapture)
			}
			mu.Unlock()

		case "input":
			if msg.Data != nil {
				handleInput(msg.Data)
			}

		case "stop_session":
			mu.Lock()
			if capturing {
				capturing = false
				close(stopCapture)
			}
			mu.Unlock()
		}
	}
}

func captureAndStream(ws *websocket.Conn, sessionID string, stop <-chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			frame, err := captureScreen()
			if err != nil {
				continue
			}
			encoded := base64.StdEncoding.EncodeToString(frame)
			ws.WriteJSON(map[string]string{
				"type":       "frame",
				"session_id": sessionID,
				"data":       encoded,
			})
		}
	}
}

func captureScreen() ([]byte, error) {
	if runtime.GOOS == "windows" {
		return captureScreenWindows()
	}
	return captureScreenLinux()
}

func captureScreenWindows() ([]byte, error) {
	ps := `Add-Type -AssemblyName System.Windows.Forms
$screen = [System.Windows.Forms.Screen]::PrimaryScreen
$bounds = $screen.Bounds
$bmp = New-Object System.Drawing.Bitmap($bounds.Width, $bounds.Height)
$graphics = [System.Drawing.Graphics]::FromImage($bmp)
$graphics.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
$ms = New-Object System.IO.MemoryStream
$bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
$bytes = $ms.ToArray()
$ms.Dispose()
$graphics.Dispose()
$bmp.Dispose()
[Convert]::ToBase64String($bytes)`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Output()
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
}

func captureScreenLinux() ([]byte, error) {
	out, err := exec.Command("sh", "-c", "which xdotool scrot gnome-screenshot 2>/dev/null").Output()
	if err != nil {
		return createPlaceholderFrame("Linux Agent - Screen Capture")
	}
	tools := strings.Fields(string(out))
	for _, t := range tools {
		switch {
		case strings.Contains(t, "scrot"):
			img, err := captureWithScrot()
			if err == nil {
				return img, nil
			}
		}
	}
	return createPlaceholderFrame("Linux Agent - Screen Capture")
}

func captureWithScrot() ([]byte, error) {
	tmp := filepath.Join(os.TempDir(), "cyverra_screen.png")
	if err := exec.Command("scrot", tmp).Run(); err != nil {
		return nil, err
	}
	defer os.Remove(tmp)
	data, err := os.ReadFile(tmp)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func createPlaceholderFrame(text string) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 40, B: 50, A: 255})
		}
	}
	_ = text
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 50})
	return buf.Bytes(), nil
}

func handleInput(data json.RawMessage) {
	var input struct {
		Type string `json:"type"`
		X    int    `json:"x"`
		Y    int    `json:"y"`
		Key  string `json:"key"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return
	}
	if runtime.GOOS != "windows" {
		return
	}
	switch input.Type {
	case "mousemove":
		exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			fmt.Sprintf("Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d,%d)", input.X, input.Y)).Run()
	case "mousedown", "mouseup":
		exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			fmt.Sprintf("Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d,%d)", input.X, input.Y)).Run()
	case "click":
		exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			fmt.Sprintf("Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d,%d); Add-Type -AssemblyName System.Windows.Forms -ReferencedAssemblies System.Drawing; [System.Windows.Forms.SendKeys]::SendWait('')", input.X, input.Y)).Run()
	case "keydown":
		handleKeyboard(input.Key, true)
	case "keyup":
		handleKeyboard(input.Key, false)
	}
}

func handleKeyboard(key string, down bool) {
	if runtime.GOOS != "windows" {
		return
	}
	if down {
		exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			fmt.Sprintf("Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(\"%s\")", key)).Run()
	}
}

func manageMeshCentralAgent(cfg config) {
	mcConfig := fetchMCConfig(cfg)
	if mcConfig == nil || !mcConfig.Enabled || mcConfig.ServerURL == "" {
		return
	}
	logMessage("meshcentral integration enabled, managing MC agent")

	mcBin := findMCAgentBinary()
	if mcBin == "" {
		logMessage("meshcentral agent not found, attempting download...")
		if err := downloadMCAgent(mcConfig); err != nil {
			logMessage("meshcentral agent download failed: " + err.Error())
			return
		}
		mcBin = findMCAgentBinary()
	}
	if mcBin == "" {
		logMessage("meshcentral agent binary not found after download")
		return
	}

	logMessage("meshcentral agent found: " + mcBin)
	for {
		ensureMCAgentRunning(mcBin, mcConfig)
		time.Sleep(60 * time.Second)
		mcConfig = fetchMCConfig(cfg)
		if mcConfig == nil || !mcConfig.Enabled {
			stopMCAgent()
			return
		}
	}
}

type mcConfigData struct {
	Enabled    bool   `json:"enabled"`
	ServerURL  string `json:"server_url"`
	APIKey     string `json:"api_key"`
	AgentGroup string `json:"agent_group"`
}

func fetchMCConfig(cfg config) *mcConfigData {
	var result mcConfigData
	if err := get(cfg.API+"/api/v1/agent/meshcentral-config", cfg.Credential, &result); err != nil {
		return nil
	}
	return &result
}

var mcAgentProcess *os.Process

func findMCAgentBinary() string {
	paths := []string{
		"/usr/local/bin/meshagent",
		"/usr/bin/meshagent",
		filepath.Join(os.TempDir(), "meshagent"),
	}
	if runtime.GOOS == "windows" {
		paths = []string{
			filepath.Join(os.Getenv("ProgramFiles"), "MeshCentral", "meshagent.exe"),
			filepath.Join(os.TempDir(), "meshagent.exe"),
			"C:\\Program Files\\MeshCentral\\meshagent.exe",
		}
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func downloadMCAgent(cfg *mcConfigData) error {
	arch := "4"
	if runtime.GOARCH == "arm64" {
		arch = "25"
	} else if runtime.GOARCH == "386" {
		arch = "3"
	}
	if runtime.GOOS == "linux" {
		arch = "6"
		if runtime.GOARCH == "arm64" {
			arch = "26"
		} else if runtime.GOARCH == "386" {
			arch = "5"
		}
	}

	url := strings.TrimRight(cfg.ServerURL, "/") + "/meshagents?id=" + arch
	if cfg.AgentGroup != "" {
		url += "&meshid=" + cfg.AgentGroup
	}

	var dest string
	if runtime.GOOS == "windows" {
		dest = filepath.Join(os.TempDir(), "meshagent.exe")
	} else {
		dest = filepath.Join(os.TempDir(), "meshagent")
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		os.Chmod(dest, 0755)
	}
	logMessage("meshcentral agent downloaded to " + dest)
	return nil
}

func ensureMCAgentRunning(bin string, cfg *mcConfigData) {
	if mcAgentProcess != nil {
		if err := mcAgentProcess.Signal(syscall.Signal(0)); err == nil {
			return
		}
		mcAgentProcess = nil
	}

	mshContent := fmt.Sprintf("MeshServer=%s\nMeshID=%s\n", cfg.ServerURL, cfg.AgentGroup)
	mshPath := bin + ".msh"
	os.WriteFile(mshPath, []byte(mshContent), 0600)

	args := []string{"-url", cfg.ServerURL}
	if cfg.AgentGroup != "" {
		args = append(args, "-meshid", cfg.AgentGroup)
	}
	args = append(args, "-mshfile", mshPath)

	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		logMessage("meshcentral agent start failed: " + err.Error())
		return
	}
	mcAgentProcess = cmd.Process
	logMessage("meshcentral agent started (pid=" + fmt.Sprint(cmd.Process.Pid) + ")")

	go cmd.Wait()
}

func stopMCAgent() {
	if mcAgentProcess != nil {
		mcAgentProcess.Kill()
		mcAgentProcess = nil
		logMessage("meshcentral agent stopped")
	}
}

func executeRMMJob(cfg config, job commandJob) *commandResult {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	switch job.CommandType {
	case "powershell", "cmd", "bash":
		return executeShellCommand(ctx, job)
	case "execute_command", "execute_script":
		return executeShellCommand(ctx, job)
	case "kill_process":
		return killProcess(ctx, job)
	case "start_process":
		return startProcess(ctx, job)
	case "start_service":
		return controlService(ctx, job, "start")
	case "stop_service":
		return controlService(ctx, job, "stop")
	case "restart_service":
		return controlService(ctx, job, "restart")
	case "reboot":
		return systemAction(ctx, job, "reboot")
	case "shutdown":
		return systemAction(ctx, job, "shutdown")
	case "lock_workstation":
		return systemAction(ctx, job, "lock")
	case "logoff_user":
		return systemAction(ctx, job, "logoff")
	case "retrieve_file":
		return retrieveFile(cfg, ctx, job)
	case "upload_file":
		return uploadFile(ctx, job)
	case "delete_file":
		return deleteFile(ctx, job)
	case "rename_file":
		return renameFile(ctx, job)
	case "browse_filesystem":
		return browseFilesystem(ctx, job)
	default:
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "unknown command_type: " + job.CommandType}
	}
}

func executeShellCommand(ctx context.Context, job commandJob) *commandResult {
	name, args := "sh", []string{"-c", job.Command}
	switch job.CommandType {
	case "powershell":
		if runtime.GOOS == "windows" {
			name = "powershell.exe"
			args = []string{"-NoProfile", "-NonInteractive", "-Command", job.Command}
		} else {
			name = "sh"
			args = []string{"-c", "pwsh -NoProfile -NonInteractive -Command '" + job.Command + "'"}
		}
	case "cmd":
		if runtime.GOOS == "windows" {
			name = "cmd.exe"
			args = []string{"/C", job.Command}
		}
	case "bash":
		name = "bash"
		args = []string{"-c", job.Command}
	default:
		if runtime.GOOS == "windows" {
			name = "powershell.exe"
			args = []string{"-NoProfile", "-NonInteractive", "-Command", job.Command}
		}
	}
	process := exec.CommandContext(ctx, name, args...)
	output, err := process.CombinedOutput()
	status := "SUCCESS"
	if ctx.Err() == context.DeadlineExceeded {
		status = "TIMEOUT"
	} else if err != nil {
		status = "FAILED"
	}
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	}
	return &commandResult{ID: job.ID, Status: status, ExitCode: &code, Stdout: string(output)}
}

func killProcess(ctx context.Context, job commandJob) *commandResult {
	pid, _ := job.Params["pid"].(string)
	name, _ := job.Params["name"].(string)
	if pid == "" && name == "" {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "pid or name is required"}
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		if pid != "" {
			cmd = exec.CommandContext(ctx, "taskkill", "/PID", pid, "/F")
		} else {
			cmd = exec.CommandContext(ctx, "taskkill", "/IM", name, "/F")
		}
	} else {
		if pid != "" {
			cmd = exec.CommandContext(ctx, "kill", "-9", pid)
		} else {
			cmd = exec.CommandContext(ctx, "pkill", "-9", name)
		}
	}
	output, err := cmd.CombinedOutput()
	status := "SUCCESS"
	if err != nil {
		status = "FAILED"
	}
	return &commandResult{ID: job.ID, Status: status, Stdout: string(output), Stderr: fmt.Sprintf("%v", err)}
}

func startProcess(ctx context.Context, job commandJob) *commandResult {
	path, _ := job.Params["path"].(string)
	argsList, _ := job.Params["args"].([]interface{})
	if path == "" {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "path is required"}
	}
	var args []string
	for _, a := range argsList {
		if s, ok := a.(string); ok {
			args = append(args, s)
		}
	}
	cmd := exec.CommandContext(ctx, path, args...)
	output, err := cmd.CombinedOutput()
	status := "SUCCESS"
	if err != nil {
		status = "FAILED"
	}
	return &commandResult{ID: job.ID, Status: status, Stdout: string(output), Stderr: fmt.Sprintf("%v", err)}
}

func controlService(ctx context.Context, job commandJob, action string) *commandResult {
	serviceName, _ := job.Params["service"].(string)
	if serviceName == "" {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "service name is required"}
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		switch action {
		case "start":
			cmd = exec.CommandContext(ctx, "net", "start", serviceName)
		case "stop":
			cmd = exec.CommandContext(ctx, "net", "stop", serviceName)
		case "restart":
			stopCmd := exec.CommandContext(ctx, "net", "stop", serviceName)
			_, _ = stopCmd.CombinedOutput()
			cmd = exec.CommandContext(ctx, "net", "start", serviceName)
		}
	} else {
		cmd = exec.CommandContext(ctx, "systemctl", action, serviceName)
	}
	output, err := cmd.CombinedOutput()
	status := "SUCCESS"
	if err != nil {
		status = "FAILED"
	}
	return &commandResult{ID: job.ID, Status: status, Stdout: string(output), Stderr: fmt.Sprintf("%v", err)}
}

func systemAction(ctx context.Context, job commandJob, action string) *commandResult {
	delaySec, _ := job.Params["delay"].(float64)
	delay := fmt.Sprintf("%d", int(delaySec))

	var cmd *exec.Cmd
	switch action {
	case "reboot":
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "shutdown", "/r", "/t", delay)
		} else {
			cmd = exec.CommandContext(ctx, "reboot")
		}
	case "shutdown":
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "shutdown", "/s", "/t", delay)
		} else {
			cmd = exec.CommandContext(ctx, "shutdown", "-h", "+0")
		}
	case "lock":
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "rundll32.exe", "user32.dll,LockWorkStation")
		} else {
			return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "lock_workstation not supported on Linux"}
		}
	case "logoff":
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "shutdown", "/l")
		} else {
			cmd = exec.CommandContext(ctx, "loginctl", "terminate-user", os.Getenv("USER"))
		}
	default:
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "unknown system action: " + action}
	}
	output, err := cmd.CombinedOutput()
	status := "SUCCESS"
	if err != nil {
		status = "FAILED"
	}
	return &commandResult{ID: job.ID, Status: status, Stdout: string(output), Stderr: fmt.Sprintf("%v", err)}
}

func retrieveFile(cfg config, ctx context.Context, job commandJob) *commandResult {
	filePath, _ := job.Params["path"].(string)
	if filePath == "" {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "path is required"}
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: err.Error()}
	}
	fileName := filepath.Base(filePath)
	result := &commandResult{
		ID:       job.ID,
		Status:   "SUCCESS",
		FileName: fileName,
		FilePath: filePath,
		FileSize: int64Ptr(int64(len(data))),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.API+"/api/v1/agent/file-content/"+job.ID, bytes.NewReader(data))
	if err != nil {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: err.Error()}
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+cfg.Credential)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "file upload failed"}
	}
	return result
}

func int64Ptr(v int64) *int64 { return &v }

func uploadFile(ctx context.Context, job commandJob) *commandResult {
	filePath, _ := job.Params["path"].(string)
	content, _ := job.Params["content"].(string)
	if filePath == "" {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "path is required"}
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: err.Error()}
	}
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: err.Error()}
	}
	return &commandResult{ID: job.ID, Status: "SUCCESS", Stdout: "file written: " + filePath}
}

func deleteFile(ctx context.Context, job commandJob) *commandResult {
	filePath, _ := job.Params["path"].(string)
	if filePath == "" {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "path is required"}
	}
	if err := os.Remove(filePath); err != nil {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: err.Error()}
	}
	return &commandResult{ID: job.ID, Status: "SUCCESS", Stdout: "deleted: " + filePath}
}

func renameFile(ctx context.Context, job commandJob) *commandResult {
	oldPath, _ := job.Params["old_path"].(string)
	newPath, _ := job.Params["new_path"].(string)
	if oldPath == "" || newPath == "" {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: "old_path and new_path are required"}
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: err.Error()}
	}
	return &commandResult{ID: job.ID, Status: "SUCCESS", Stdout: "renamed: " + oldPath + " -> " + newPath}
}

func browseFilesystem(ctx context.Context, job commandJob) *commandResult {
	dirPath, _ := job.Params["path"].(string)
	if dirPath == "" {
		if runtime.GOOS == "windows" {
			dirPath = "C:\\"
		} else {
			dirPath = "/"
		}
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return &commandResult{ID: job.ID, Status: "FAILED", Stderr: err.Error()}
	}
	result := []map[string]any{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		item := map[string]any{
			"name":  entry.Name(),
			"isDir": entry.IsDir(),
			"size":  info.Size(),
			"mode":  info.Mode().String(),
			"modTime": info.ModTime().Format(time.RFC3339),
		}
		result = append(result, item)
	}
	output, _ := json.Marshal(result)
	return &commandResult{ID: job.ID, Status: "SUCCESS", Stdout: string(output)}
}

type commandResult struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ExitCode    *int   `json:"exit_code,omitempty"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	FileName    string `json:"file_name,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
	FileSize    *int64 `json:"file_size,omitempty"`
}

func get(url, credential string, response any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	client := &http.Client{Timeout: 30 * time.Second}
	result, err := client.Do(req)
	if err != nil {
		return err
	}
	defer result.Body.Close()
	if result.StatusCode == 204 {
		return nil
	}
	if result.StatusCode >= 300 {
		return fmt.Errorf("api returned %s", result.Status)
	}
	return json.NewDecoder(io.LimitReader(result.Body, 1<<20)).Decode(response)
}
func sendMetrics(cfg config) {
	data := metrics{}
	if total, used, ok := linuxMemory(); ok {
		data.MemoryTotalBytes = &total
		data.MemoryUsedBytes = &used
	}
	if cpu, ok := linuxCPU(); ok {
		data.CPUPercent = &cpu
	}
	var response map[string]any
	if err := post(cfg.API+"/api/v1/agent/metrics", cfg.Credential, data, &response); err != nil {
		fmt.Fprintf(os.Stderr, "metrics unavailable: %v\n", err)
	}
}
func linuxCPU() (float64, bool) {
	if runtime.GOOS != "linux" {
		return 0, false
	}
	read := func() (uint64, uint64, bool) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0, false
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 5 && fields[0] == "cpu" {
				var total, idle uint64
				for _, value := range fields[1:] {
					parsed, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						return 0, 0, false
					}
					total += parsed
				}
				idle = mustUint(fields[4])
				return total, idle, true
			}
		}
		return 0, 0, false
	}
	firstTotal, firstIdle, ok := read()
	if !ok {
		return 0, false
	}
	time.Sleep(200 * time.Millisecond)
	secondTotal, secondIdle, ok := read()
	if !ok || secondTotal <= firstTotal {
		return 0, false
	}
	usage := float64((secondTotal-firstTotal)-(secondIdle-firstIdle)) / float64(secondTotal-firstTotal) * 100
	return usage, true
}
func mustUint(value string) uint64 { parsed, _ := strconv.ParseUint(value, 10, 64); return parsed }
func sendInventory(cfg config) {
	hostname, _ := os.Hostname()
	data := inventory{Hostname: hostname, Platform: runtime.GOOS, OSName: runtime.GOOS, Architecture: runtime.GOARCH, CPUCores: runtime.NumCPU(), Network: collectNetwork(), Disks: collectDisks(), Software: collectSoftware(), Services: collectServices()}
	if total, used, ok := linuxMemory(); ok {
		data.MemoryTotal, data.MemoryUsed = total, used
	}
	var response map[string]any
	if err := post(cfg.API+"/api/v1/agent/inventory", cfg.Credential, data, &response); err != nil {
		fmt.Fprintf(os.Stderr, "inventory unavailable: %v\n", err)
	}
}
func runInventoryCommand(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return string(output)
}
func collectDisks() []diskEntry {
	if runtime.GOOS == "linux" {
		output := runInventoryCommand("df", "-P", "-B1")
		result := []diskEntry{}
		for _, line := range strings.Split(output, "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) >= 6 {
				total, _ := strconv.ParseUint(fields[1], 10, 64)
				used, _ := strconv.ParseUint(fields[2], 10, 64)
				free, _ := strconv.ParseUint(fields[3], 10, 64)
				result = append(result, diskEntry{Name: fields[5], SizeBytes: total, UsedBytes: used, FreeBytes: free, FileSystem: fields[0]})
			}
		}
		return result
	}
	if runtime.GOOS == "windows" {
		output := runInventoryCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-Volume | Where-Object DriveLetter | Select-Object DriveLetter,Size,SizeRemaining,FileSystem | ConvertTo-Csv -NoTypeInformation")
		result := []diskEntry{}
		for _, line := range strings.Split(output, "\n")[1:] {
			fields := strings.Split(strings.Trim(line, "\r\""), "\",\"")
			if len(fields) >= 4 {
				total, _ := strconv.ParseUint(fields[1], 10, 64)
				free, _ := strconv.ParseUint(fields[2], 10, 64)
				result = append(result, diskEntry{Name: fields[0], SizeBytes: total, FreeBytes: free, UsedBytes: total - free, FileSystem: fields[3]})
			}
		}
		return result
	}
	return []diskEntry{}
}
func collectSoftware() []string {
	if runtime.GOOS == "linux" {
		output := runInventoryCommand("sh", "-c", "dpkg-query -W -f='${binary:Package} ${Version}\\n' 2>/dev/null | head -n 200")
		return nonEmptyLines(output)
	}
	if runtime.GOOS == "windows" {
		output := runInventoryCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-ItemProperty HKLM:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\*,HKLM:\\Software\\Wow6432Node\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\* | Where-Object DisplayName | Select-Object -ExpandProperty DisplayName")
		return nonEmptyLines(output)
	}
	return []string{}
}
func collectServices() []serviceEntry {
	if runtime.GOOS == "linux" {
		output := runInventoryCommand("systemctl", "list-units", "--type=service", "--state=running", "--no-legend", "--plain")
		result := []serviceEntry{}
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				result = append(result, serviceEntry{Name: fields[0], Status: fields[2], Description: strings.Join(fields[3:], " ")})
			}
		}
		return result
	}
	if runtime.GOOS == "windows" {
		output := runInventoryCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-Service | Where-Object Status -eq Running | Select-Object Name,Status,DisplayName | ConvertTo-Csv -NoTypeInformation")
		result := []serviceEntry{}
		for _, line := range strings.Split(output, "\n")[1:] {
			fields := strings.Split(strings.Trim(line, "\r\""), "\",\"")
			if len(fields) >= 3 {
				result = append(result, serviceEntry{Name: fields[0], Status: fields[1], Description: fields[2]})
			}
		}
		return result
	}
	return []serviceEntry{}
}
func nonEmptyLines(output string) []string {
	result := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
func collectNetwork() []networkEntry {
	interfaces, _ := net.Interfaces()
	result := make([]networkEntry, 0, len(interfaces))
	for _, item := range interfaces {
		addresses, _ := item.Addrs()
		ips := make([]string, 0, len(addresses))
		for _, address := range addresses {
			ips = append(ips, address.String())
		}
		result = append(result, networkEntry{Name: item.Name, MAC: item.HardwareAddr.String(), IPs: ips})
	}
	return result
}
func linuxMemory() (uint64, uint64, bool) {
	if runtime.GOOS != "linux" {
		return 0, 0, false
	}
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	var total, available uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] != "" {
			value := uint64(0)
			for _, char := range fields[1] {
				value = value*10 + uint64(char-'0')
			}
			if strings.HasPrefix(line, "MemTotal:") {
				total = value * 1024
			}
			if strings.HasPrefix(line, "MemAvailable:") {
				available = value * 1024
			}
		}
	}
	if total == 0 {
		return 0, 0, false
	}
	return total, total - available, true
}
func enroll(api, token, state string) string {
	hostname, _ := os.Hostname()
	request := enrollRequest{Token: token, Hostname: hostname, DeviceUUID: deviceID(), Platform: runtime.GOOS, OSName: runtime.GOOS, Architecture: runtime.GOARCH, AgentVersion: agentVersion}
	var response enrollResponse
	if err := post(api+"/api/v1/agent/enroll", "", request, &response); err != nil {
		fatal(err.Error())
	}
	if response.Credential == "" {
		fatal("enrollment returned no credential")
	}
	data, _ := json.Marshal(config{API: api, Credential: response.Credential})
	if err := os.MkdirAll(filepath.Dir(state), 0700); err != nil {
		fatal(err.Error())
	}
	if err := os.WriteFile(state, data, 0600); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("enrolled device %s\n", response.DeviceID)
	return response.Credential
}
func sendHeartbeat(cfg config) {
	hostname, _ := os.Hostname()
	request := map[string]string{"hostname": hostname, "platform": runtime.GOOS, "os_name": runtime.GOOS, "agent_version": agentVersion}
	var response map[string]any
	if err := post(cfg.API+"/api/v1/agent/heartbeat", cfg.Credential, request, &response); err != nil {
		fmt.Fprintf(os.Stderr, "heartbeat unavailable: %v\n", err)
		return
	}
	fmt.Printf("heartbeat: %v\n", response["status"])
}
func post(url, credential string, request, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	result, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer result.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(result.Body, 1<<20))
	if result.StatusCode >= 300 {
		return fmt.Errorf("api returned %s: %s", result.Status, strings.TrimSpace(string(data)))
	}
	if err = json.Unmarshal(data, response); err != nil {
		return err
	}
	return nil
}
func deviceID() string {
	hostname, _ := os.Hostname()
	return hostname + "-" + runtime.GOOS + "-" + runtime.GOARCH
}
func defaultStatePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "cyverra", "agent.json")
	}
	return ".cyverra-agent.json"
}
func fatal(message string) { logMessage("FATAL: " + message); os.Exit(1) }
