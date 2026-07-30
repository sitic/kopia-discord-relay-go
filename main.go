// kopia-discord-relay: accepts Kopia's plain-text/HTML webhook POST at
// /api/webhooks/<id>/<token>, wraps it in the JSON payload Discord expects,
// and forwards it to the same path on discord.com.
//
// Config via environment variables:
//
//	BEARER_TOKEN  (optional)  require "Authorization: Bearer <token>"
//	LISTEN_ADDR   (optional)  default ":8199"
package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	chunkSize   = 1900 // Discord content limit is 2000; leave fence headroom
	maxBodySize = 512 * 1024
)

var (
	bearerToken = os.Getenv("BEARER_TOKEN")

	// Discord webhook path shape: /api/webhooks/<snowflake-id>/<token>
	webhookPathRe = regexp.MustCompile(`^/api/webhooks/[0-9]{1,32}/[A-Za-z0-9_-]{1,128}$`)

	brRe    = regexp.MustCompile(`(?i)<br\s*/?>`)
	blockRe = regexp.MustCompile(`(?i)</(p|div|tr|li|h[1-6])>`)
	tagRe   = regexp.MustCompile(`<[^>]+>`)

	client = &http.Client{Timeout: 15 * time.Second}
)

func stripHTML(s string) string {
	s = brRe.ReplaceAllString(s, "\n")
	s = blockRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

func postToDiscord(target, content string) error {
	payload, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "kopia-discord-relay/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord returned %s", resp.Status)
	}
	return nil
}

func authorized(r *http.Request) bool {
	if bearerToken == "" {
		return true
	}
	got := r.Header.Get("Authorization")
	want := "Bearer " + bearerToken
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Only POST /api/webhooks/<id>/<token> is accepted; the relay forwards
	// to the same path on discord.com and never to arbitrary URLs.
	if !webhookPathRe.MatchString(r.URL.Path) {
		http.Error(w, "unknown webhook path", http.StatusNotFound)
		return
	}
	target := "https://discord.com" + r.URL.Path

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize+1))
	if err != nil || len(body) == 0 || len(body) > maxBodySize {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}

	text := string(body)
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "html") {
		text = stripHTML(text)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "(empty kopia notification)"
	}

	for i := 0; i < len(text); {
		end := i + chunkSize
		if end >= len(text) {
			end = len(text)
		} else {
			// don't split a multi-byte UTF-8 rune across chunks
			for end > i && !utf8.RuneStart(text[end]) {
				end--
			}
			if end == i { // invalid UTF-8; split at the byte limit
				end = i + chunkSize
			}
		}
		if err := postToDiscord(target, "```\n"+text[i:end]+"\n```"); err != nil {
			log.Printf("discord forward failed: %v", err)
			http.Error(w, "discord forward failed", http.StatusBadGateway)
			return
		}
		i = end
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "forwarded")
}

func main() {
	if bearerToken == "" {
		log.Print("warning: BEARER_TOKEN not set; anyone who can reach this port can post")
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8199"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/", handleNotify)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	log.Printf("listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
