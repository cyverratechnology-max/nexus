package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
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
	api := flag.String("api", "http://localhost:8080", "Cyverra API base URL")
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
	ID      string `json:"id"`
	Command string `json:"command,omitempty"`
}

func pollCommands(cfg config) {
	var job commandJob
	if err := get(cfg.API+"/api/v1/agent/commands", cfg.Credential, &job); err != nil || job.ID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	name, args := "sh", []string{"-c", job.Command}
	if runtime.GOOS == "windows" {
		name, args = "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", job.Command}
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
	result := commandResult{ID: job.ID, Status: status, ExitCode: &code, Stdout: string(output)}
	_ = post(cfg.API+"/api/v1/agent/commands/"+job.ID+"/result", cfg.Credential, result, &map[string]any{})
}

type commandResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
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
