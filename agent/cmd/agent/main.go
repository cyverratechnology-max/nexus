package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

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
}
type networkEntry struct {
	Name string   `json:"name"`
	MAC  string   `json:"mac"`
	IPs  []string `json:"ips"`
}

func main() {
	api := flag.String("api", "http://localhost:8080", "Cyverra API base URL")
	token := flag.String("enrollment-token", os.Getenv("CYVERRA_ENROLLMENT_TOKEN"), "single-use enrollment token")
	state := flag.String("state", defaultStatePath(), "agent state file")
	once := flag.Bool("once", false, "send one heartbeat and exit")
	flag.Parse()
	cfg := config{API: strings.TrimRight(*api, "/")}
	if data, err := os.ReadFile(*state); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	cfg.API = strings.TrimRight(*api, "/")
	if cfg.Credential == "" {
		if *token == "" {
			fatal("enrollment token is required for first run")
		}
		cfg.Credential = enroll(cfg.API, *token, *state)
	}
	sendHeartbeat(cfg)
	sendInventory(cfg)
	if *once {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		sendHeartbeat(cfg)
		sendInventory(cfg)
	}
}
func sendInventory(cfg config) {
	hostname, _ := os.Hostname()
	data := inventory{Hostname: hostname, Platform: runtime.GOOS, OSName: runtime.GOOS, Architecture: runtime.GOARCH, CPUCores: runtime.NumCPU(), Network: collectNetwork()}
	if total, used, ok := linuxMemory(); ok {
		data.MemoryTotal, data.MemoryUsed = total, used
	}
	var response map[string]any
	if err := post(cfg.API+"/api/v1/agent/inventory", cfg.Credential, data, &response); err != nil {
		fmt.Fprintf(os.Stderr, "inventory unavailable: %v\n", err)
	}
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
	request := enrollRequest{Token: token, Hostname: hostname, DeviceUUID: deviceID(), Platform: runtime.GOOS, OSName: runtime.GOOS, Architecture: runtime.GOARCH, AgentVersion: "0.1.0"}
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
	request := map[string]string{"hostname": hostname, "platform": runtime.GOOS, "os_name": runtime.GOOS, "agent_version": "0.1.0"}
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
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
