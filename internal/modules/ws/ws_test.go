package ws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/UzStack/jst-go/internal/modules/auth"
	"github.com/UzStack/jst-go/internal/modules/ws"
	"github.com/UzStack/jst-go/internal/shared/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	gorilla "github.com/gorilla/websocket"
)

func newWSTestServer(t *testing.T) (*httptest.Server, *auth.TokenIssuer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	tokens := auth.NewTokenIssuer(config.JWTConfig{
		Secret: "test-secret-long-enough-1234567890", AccessTTL: time.Minute,
		RefreshTTL: time.Hour, Issuer: "ws-test",
	})

	hub := ws.NewHub()
	go hub.Run(ctx)

	r := gin.New()
	ws.RegisterRoutes(r.Group("/api/v1"), ws.NewHandler(hub, tokens, []string{"*"}))

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, tokens
}

func wsURL(httpURL, token string) string {
	u := "ws" + strings.TrimPrefix(httpURL, "http") + "/api/v1/ws"
	if token != "" {
		u += "?token=" + token
	}
	return u
}

func mintToken(t *testing.T, tokens *auth.TokenIssuer) string {
	t.Helper()
	tok, _, err := tokens.NewAccessToken(uuid.New(), "user")
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return tok
}

func TestConnect_RequiresAuth(t *testing.T) {
	srv, _ := newWSTestServer(t)

	_, resp, err := gorilla.DefaultDialer.Dial(wsURL(srv.URL, ""), nil)
	if err == nil {
		t.Fatal("expected handshake to fail without a token")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}
	_ = resp.Body.Close()
}

func TestBroadcast_DeliversToAllClients(t *testing.T) {
	srv, tokens := newWSTestServer(t)

	c1, resp1, err := gorilla.DefaultDialer.Dial(wsURL(srv.URL, mintToken(t, tokens)), nil)
	if err != nil {
		t.Fatalf("dial c1: %v", err)
	}
	_ = resp1.Body.Close()
	defer c1.Close()
	c2, resp2, err := gorilla.DefaultDialer.Dial(wsURL(srv.URL, mintToken(t, tokens)), nil)
	if err != nil {
		t.Fatalf("dial c2: %v", err)
	}
	_ = resp2.Body.Close()
	defer c2.Close()

	// Give both clients time to register in the hub before broadcasting.
	time.Sleep(100 * time.Millisecond)

	if err := c1.WriteMessage(gorilla.TextMessage, []byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}

	for i, c := range []*gorilla.Conn{c1, c2} {
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		var msg ws.Message
		if err := c.ReadJSON(&msg); err != nil {
			t.Fatalf("client %d read: %v", i+1, err)
		}
		if msg.Type != "message" || msg.Body != "hello" {
			t.Errorf("client %d got %+v, want type=message body=hello", i+1, msg)
		}
		if msg.From == "" {
			t.Errorf("client %d: missing sender id", i+1)
		}
	}
}
