package server

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hestia/internal/config"
)

func testListenHost() string {
	// Binding all interfaces in a new test executable can show a Windows
	// Firewall prompt. Opt in when testing the wildcard-specific conflict.
	if os.Getenv("HESTIA_TEST_WILDCARD_LISTENER") == "1" {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

func occupiedLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener.Addr().(*net.TCPAddr).Port
}

func TestStartListenerSkipsOccupiedLoopbackPort(t *testing.T) {
	occupied := occupiedLoopbackPort(t)
	srv, _ := libraryFixture(t)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cfg.Close)
	if err := cfg.Update(func(c *config.Config) {
		c.Listen = testListenHost()
		c.Port = occupied
	}); err != nil {
		t.Fatal(err)
	}
	srv.cfg = cfg
	t.Cleanup(srv.StopListener)

	if err := srv.StartListener(); err != nil {
		t.Fatal(err)
	}
	if got := srv.Port(); got <= occupied || got > occupied+20 {
		t.Fatalf("actual port = %d, want a fallback after occupied port %d", got, occupied)
	}
	port := srv.Port()
	if err := srv.StartListener(); err != nil || srv.Port() != port {
		t.Fatalf("repeated start changed the fallback port: %d, err=%v", srv.Port(), err)
	}
	wantURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if got := srv.LocalURL(); got != wantURL {
		t.Fatalf("local URL = %q, want %q", got, wantURL)
	}
	assertListenerHomepage(t, srv)
}

func assertListenerHomepage(t *testing.T, srv *Server) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(srv.LocalURL() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Equal(body, srv.index) {
		t.Fatalf("expected Hestia homepage, status = %d", response.StatusCode)
	}
}

func TestRestartListenerKeepsWorkingPortWhenLoopbackIsOccupied(t *testing.T) {
	occupied := occupiedLoopbackPort(t)
	srv, _ := libraryFixture(t)
	t.Cleanup(srv.StopListener)
	host := testListenHost()
	if err := srv.RestartListener(host, 0); err != nil {
		t.Fatal(err)
	}
	active := srv.Port()
	if err := srv.RestartListener(host, occupied); err == nil {
		t.Fatal("bind succeeded despite an occupied loopback port")
	}
	if !srv.Running() || srv.Port() != active || active == 0 {
		t.Fatalf("old listener port %d was not kept: port=%d", active, srv.Port())
	}
	assertListenerHomepage(t, srv)
}
